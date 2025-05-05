package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func initDB() {
	// Get environment variables
	dbUser := os.Getenv("DB_USER")
	dbPass := os.Getenv("DB_PASS")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST") // Public IP: 34.122.48.1

	if dbUser == "" || dbPass == "" || dbName == "" || dbHost == "" {
		fmt.Printf("Missing environment variables:\nDB_USER: %s\nDB_PASS: %s\nDB_NAME: %s\nDB_HOST: %s\n",
			dbUser, dbPass, dbName, dbHost)
		return
	}

	// Use Public IP connection
	dsn := fmt.Sprintf("host=%s port=5432 user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbUser, dbPass, dbName)

	fmt.Printf("Attempting to connect with DSN: %s\n", dsn)

	var err error
	db, err = sql.Open("postgres", dsn)
	if err != nil {
		fmt.Printf("Error opening database connection: %v\n", err)
		return
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	// Try to ping the database with retries
	for i := 0; i < 5; i++ {
		err = db.Ping()
		if err == nil {
			break
		}
		fmt.Printf("Attempt %d: Error connecting to database: %v\n", i+1, err)
		time.Sleep(time.Second * 2)
	}

	if err != nil {
		fmt.Printf("Final error connecting to database: %v\n", err)
		return
	}

	fmt.Println("Successfully connected to database")
}

type Traveler struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type AirportInfo struct {
	Airport string `json:"airport"`
	Date    string `json:"date"`
	Time    string `json:"time"`
}

type ItineraryRequest struct {
	Countries []string    `json:"countries"`
	Arrival   AirportInfo `json:"arrival"`
	Departure AirportInfo `json:"departure"`
	Travelers []Traveler  `json:"travelers"`
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Password string `json:"password,omitempty"`
}

func main() {
	// Add startup delay to allow Cloud Run to initialize
	time.Sleep(5 * time.Second)

	initDB()
	// Set Gin to release mode in production
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize Gin router
	r := gin.Default()

	// Add middleware for CORS
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Define routes
	r.POST("/itinerary", handleItineraryRequest)
	r.GET("/health", func(c *gin.Context) {
		// Check database connection in health check
		if db == nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "database not connected",
			})
			return
		}

		// Try to ping the database with a timeout
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		err := db.PingContext(ctx)
		if err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "database connection failed",
				"error":  err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
		})
	})
	r.POST("/signup", signupHandler)
	r.POST("/signin", signinHandler)

	// Get port from environment variable or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Start the server
	fmt.Printf("Server is running on port %s\n", port)
	if err := r.Run(":" + port); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

func handleItineraryRequest(c *gin.Context) {
	var requestBody ItineraryRequest
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error marshaling JSON"})
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://gsc2025-sps-418414887688.us-central1.run.app/run", bytes.NewBuffer(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating request"})
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client and send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error sending request"})
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading response"})
		return
	}

	// Return the response
	c.Data(resp.StatusCode, "application/json", body)
}

func signupHandler(c *gin.Context) {
	var req User
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "username and password required"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to hash password"})
		return
	}
	_, err = db.Exec("INSERT INTO users (username, password) VALUES ($1, $2)", req.Username, string(hash))
	if err != nil {
		c.JSON(500, gin.H{"error": "failed to create user"})
		return
	}
	c.JSON(200, gin.H{"message": "user created"})
}

func signinHandler(c *gin.Context) {
	var req User
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(400, gin.H{"error": "username and password required"})
		return
	}
	var hash string
	err := db.QueryRow("SELECT password FROM users WHERE username=$1", req.Username).Scan(&hash)
	if err != nil {
		c.JSON(401, gin.H{"error": "invalid username or password"})
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)) != nil {
		c.JSON(401, gin.H{"error": "invalid username or password"})
		return
	}
	c.JSON(200, gin.H{"message": "login success"})
}
