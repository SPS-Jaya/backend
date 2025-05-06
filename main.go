package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	_ "github.com/jackc/pgx/v5/stdlib"
	"golang.org/x/crypto/bcrypt"
)

var db *sql.DB

// initDB menginisialisasi koneksi via Cloud SQL Connector
func initDB(ctx context.Context) *sql.DB {
	// Ambil env vars
	dbUser := os.Getenv("DB_USER")                        // e.g. "postgres"
	dbPass := os.Getenv("DB_PASS")                        // DB password
	dbName := os.Getenv("DB_NAME")                        // e.g. "sps_db"
	instanceConn := os.Getenv("INSTANCE_CONNECTION_NAME") // e.g. "project:region:instance"
	usePrivate := os.Getenv("PRIVATE_IP")                 // "true" jika pakai private IP (opsional)

	// Validasi
	if dbUser == "" || dbPass == "" || dbName == "" || instanceConn == "" {
		log.Fatal("Env vars DB_USER, DB_PASS, DB_NAME, INSTANCE_CONNECTION_NAME must be set")
	}

	// Build DSN dasar untuk pgx
	dsn := fmt.Sprintf("user=%s password=%s dbname=%s", dbUser, dbPass, dbName)
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		log.Fatalf("pgx.ParseConfig: %v", err)
	}

	// Setup Cloud SQL Connector dialer dengan opsi
	var opts []cloudsqlconn.Option
	if usePrivate != "" {
		opts = append(opts, cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPrivateIP()))
	}
	opts = append(opts, cloudsqlconn.WithLazyRefresh())

	dialer, err := cloudsqlconn.NewDialer(ctx, opts...)
	if err != nil {
		log.Fatalf("cloudsqlconn.NewDialer: %v", err)
	}

	// Override dial function untuk pgx
	config.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return dialer.Dial(ctx, instanceConn)
	}

	// Daftarkan config ke driver pgx
	connStr := stdlib.RegisterConnConfig(config)

	// Buka pool koneksi
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatalf("sql.Open: %v", err)
	}

	// Test koneksi
	if err = db.PingContext(ctx); err != nil {
		log.Fatalf("db.Ping: %v", err)
	}

	log.Println("✔️ Connected to Cloud SQL via Connector")
	return db
}

func main() {
	// Context untuk connector
	ctx := context.Background()

	// Inisialisasi DB
	db = initDB(ctx)
	defer db.Close()

	// Set Gin mode
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	// CORS middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Routes
	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.POST("/signup", signupHandler)
	r.POST("/signin", signinHandler)
	r.POST("/itinerary", handleItineraryRequest)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Printf("🚀 Server listening on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func signupHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to hash password"})
		return
	}

	_, err = db.ExecContext(c, `
		INSERT INTO users (username, password)
		VALUES ($1, $2)
	`, req.Username, string(hash))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "user created"})
}

func signinHandler(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Username == "" || req.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}

	var storedHash string
	err := db.QueryRowContext(c, `
		SELECT password FROM users WHERE username = $1
	`, req.Username).Scan(&storedHash)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	if bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(req.Password)) != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "login success"})
}

func handleItineraryRequest(c *gin.Context) {
	var requestBody map[string]interface{}
	if err := c.ShouldBindJSON(&requestBody); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error marshaling JSON"})
		return
	}

	req, err := http.NewRequest("POST", "https://gsc2025-sps-418414887688.us-central1.run.app/run", bytes.NewBuffer(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error creating request"})
		return
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error sending request"})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error reading response"})
		return
	}

	c.Data(resp.StatusCode, "application/json", body)
}
