package config

import (
	"log"
	"os"
	

	"github.com/joho/godotenv"
)

// Config holds all configuration for the application
type Config struct {
	DBSource       string
	ServerPort     string
	MigrationURL   string // For file-based migrations: "file://./migrations"
	FrontendURL    string // URL for the frontend
	// Add other configurations like JWT secret, etc.
}

// LoadConfig loads configuration from environment variables
// Path is the directory where .env might be located (e.g., ".")
func LoadConfig(path string) (*Config, error) {
	if err := godotenv.Load(path + "/.env"); err != nil {
		log.Println("No .env file found or error loading, relying on OS environment variables")
	}

	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPassword := getEnv("DB_PASSWORD", "password")
	dbName := getEnv("DB_NAME", "inventory_db")
	dbSSLMode := getEnv("DB_SSLMODE", "disable")
	frontendURL := getEnv("FRONTEND_URL", "http://localhost:5173") // Default for Vite React dev

	// Example: "postgresql://user:password@host:port/dbname?sslmode=disable"
	dbSource := "postgresql://" + dbUser + ":" + dbPassword + "@" + dbHost + ":" + dbPort + "/" + dbName + "?sslmode=" + dbSSLMode

	serverPort := getEnv("SERVER_PORT", "8080")
	migrationURL := getEnv("MIGRATION_URL", "file://./migrations") // Default to local file system migrations

	return &Config{
		DBSource:     dbSource,
		ServerPort:   serverPort,
		MigrationURL: migrationURL,
		FrontendURL:   frontendURL,
	}, nil
}

// Helper function to get an environment variable or return a default value
func getEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}


