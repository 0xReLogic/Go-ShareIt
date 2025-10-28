package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	// Load configuration
	loadConfig()

	// Setup handlers
	setupRoutes()

	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("uploads", os.ModePerm); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	// Start a background goroutine to clean up expired files
	go cleanupExpiredFiles()

	// Determine the address to listen on
	addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)

	log.Printf("Server starting on http://%s:%d",
		strings.Replace(config.Server.Host, defaultHost, "localhost", 1),
		config.Server.Port)
	log.Printf("Admin dashboard available at http://%s:%d/admin",
		strings.Replace(config.Server.Host, defaultHost, "localhost", 1),
		config.Server.Port)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// setupRoutes configures all HTTP routes
func setupRoutes() {
	// Web pages
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "web/index.html")
	})

	http.HandleFunc("/multi", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/multi-upload.html")
	})

	http.HandleFunc("/e2e", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/e2e-upload.html")
	})

	http.HandleFunc("/decrypt/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/decrypt.html")
	})

	// File handling endpoints
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/files/", downloadHandler)

	// API endpoints
	http.HandleFunc("/api/stats", statsHandler)
	http.HandleFunc("/api/files", basicAuth(filesListHandler, AdminUsername, AdminPassword))
	http.HandleFunc("/api/files/", basicAuth(fileActionHandler, AdminUsername, AdminPassword))

	// Admin dashboard
	http.HandleFunc("/admin", basicAuth(func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/admin.html")
	}, AdminUsername, AdminPassword))
}
