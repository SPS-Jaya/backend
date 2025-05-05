package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"database/sql"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

func initDB() {
	pw := os.Getenv("dbpw")
	connStr := fmt.Sprintf("host=34.122.48.1 port=5432 user=postgres password=%s dbname=postgres sslmode=disable", pw)
	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		panic(err)
	}
	if err = db.Ping(); err != nil {
		panic(err)
	}
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
	r.Run(":" + port)
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
