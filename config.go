package main

import (
	"encoding/json"
	"log"
	"os"
)

// Configuration structure
type Config struct {
	Server struct {
		Port int    `json:"port"`
		Host string `json:"host"`
	} `json:"server"`
	Files struct {
		DefaultExpirationMinutes int      `json:"defaultExpirationMinutes"`
		AllowedExtensions        []string `json:"allowedExtensions"`
	} `json:"files"`
	Admin struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"admin"`
}

// Global configuration variables
var (
	config            Config
	DefaultExpiration = 5        // 5 minutes
	AdminUsername     = "admin"  // Default admin username
	AdminPassword     = "admin123" // Default admin password
	allowedExtensions = []string{
		".jpg", ".jpeg", ".png", ".gif", ".pdf", ".doc", ".docx",
		".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".zip", ".rar",
		".7z", ".mp3", ".mp4", ".avi", ".mov", encryptedExt,
	}
)

// loadConfig loads the configuration from config.json file
func loadConfig() {
	// Try to read the config file
	configFile, err := os.Open("config.json")
	if err != nil {
		log.Println("Could not open config file:", err)
		log.Println("Using default configuration")
		setDefaultConfig()
		return
	}
	defer configFile.Close()

	// Decode the JSON config file
	decoder := json.NewDecoder(configFile)
	if err := decoder.Decode(&config); err != nil {
		log.Println("Error parsing config file:", err)
		log.Println("Using default configuration")
		setDefaultConfig()
		return
	}

	// Update global variables with config values
	DefaultExpiration = config.Files.DefaultExpirationMinutes
	AdminUsername = config.Admin.Username
	AdminPassword = config.Admin.Password

	if len(config.Files.AllowedExtensions) > 0 {
		allowedExtensions = config.Files.AllowedExtensions
	}

	// Load encryption key from environment variable or config
	loadEncryptionKey()

	log.Println("Configuration loaded successfully")
}

// loadEncryptionKey loads the encryption key from environment variable
func loadEncryptionKey() {
	envKey := os.Getenv("SHAREIT_ENCRYPTION_KEY")
	if envKey == "" {
		log.Fatal("SHAREIT_ENCRYPTION_KEY environment variable not set. See .env.example for setup instructions.")
	}

	encryptionKey = []byte(envKey)
	if len(encryptionKey) != 32 {
		log.Fatal("SHAREIT_ENCRYPTION_KEY must be exactly 32 characters")
	}

	log.Println("Encryption key loaded successfully")
}

// setDefaultConfig sets default values for the configuration
func setDefaultConfig() {
	config.Server.Port = 8081
	config.Server.Host = defaultHost
	config.Files.DefaultExpirationMinutes = 5
	config.Files.AllowedExtensions = allowedExtensions
	config.Admin.Username = AdminUsername
	config.Admin.Password = AdminPassword
}
