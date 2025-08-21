package main

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Configuration structure
type Config struct {
	Server struct {
		Port int    `json:"port"`
		Host string `json:"host"`
	} `json:"server"`
	Files struct {
		MaxSizeMB                int      `json:"maxSizeMB"`
		DefaultExpirationMinutes int      `json:"defaultExpirationMinutes"`
		AllowedExtensions        []string `json:"allowedExtensions"`
	} `json:"files"`
	Admin struct {
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"admin"`
}

// Global variables for state management
var (
	config     Config
	fileTokens = make(map[string]*fileInfo)
	mutex      = &sync.Mutex{}

	// Default configuration values
	MaxFileSize       int64 = 1024 * 1024 * 100 // 100 MB
	DefaultExpiration       = 5                 // 5 minutes
	AdminUsername           = "admin"           // Default admin username
	AdminPassword           = "admin123"        // Default admin password
	allowedExtensions       = []string{
		".jpg", ".jpeg", ".png", ".gif", ".pdf", ".doc", ".docx",
		".xls", ".xlsx", ".ppt", ".pptx", ".txt", ".zip", ".rar",
		".7z", ".mp3", ".mp4", ".avi", ".mov", ".encrypted",
	}

	// Encryption key for token security (32 bytes for AES-256)
	encryptionKey = []byte("12345678901234567890123456789012") // Exactly 32 bytes
)

// fileInfo stores information about a shared file
type fileInfo struct {
	Token        string    `json:"token"`        // The token used to access the file
	Path         string    `json:"path"`         // Path to the file on disk
	OriginalName string    `json:"originalName"` // Original filename
	CreatedAt    time.Time `json:"createdAt"`    // When the file was uploaded
	ExpiresAt    time.Time `json:"expiresAt"`    // When the file link expires
	Password     string    `json:"-"`            // Optional password hash (if protected)
	IsProtected  bool      `json:"isProtected"`  // Whether the file is password protected
	Size         int64     `json:"size"`         // File size in bytes
	OriginalSize int64     `json:"originalSize"` // Original file size before compression
	IsCompressed bool      `json:"isCompressed"` // Whether the file is compressed
	IsEncrypted  bool      `json:"isEncrypted"`  // Whether the file is end-to-end encrypted
	URL          string    `json:"url"`          // Shareable URL
}

// uploadResponse is the JSON response for successful uploads
type uploadResponse struct {
	Success      bool   `json:"success"`
	URL          string `json:"url"`
	ExpiresIn    int    `json:"expiresIn"` // Minutes until expiration
	IsProtected  bool   `json:"isProtected"`
	OriginalName string `json:"originalName"`
	Size         int64  `json:"size"`         // File size in bytes
	OriginalSize int64  `json:"originalSize"` // Original file size before compression
	IsCompressed bool   `json:"isCompressed"` // Whether the file is compressed
	IsEncrypted  bool   `json:"isEncrypted"`  // Whether the file is end-to-end encrypted
}

// errorResponse is the JSON response for errors
type errorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func main() {
	// Load configuration
	loadConfig()

	// Setup handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})

	// Multi-file upload page
	http.HandleFunc("/multi", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "multi-upload.html")
	})

	// End-to-end encrypted upload page
	http.HandleFunc("/e2e", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "e2e-upload.html")
	})

	// Decryption page for encrypted files
	http.HandleFunc("/decrypt/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "decrypt.html")
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
		http.ServeFile(w, r, "admin.html")
	}, AdminUsername, AdminPassword))

	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("uploads", os.ModePerm); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	// Start a background goroutine to clean up expired files
	go cleanupExpiredFiles()

	// Determine the address to listen on
	addr := fmt.Sprintf("%s:%d", config.Server.Host, config.Server.Port)

	log.Printf("Server starting on http://%s:%d",
		strings.Replace(config.Server.Host, "0.0.0.0", "localhost", 1),
		config.Server.Port)
	log.Printf("Admin dashboard available at http://%s:%d/admin",
		strings.Replace(config.Server.Host, "0.0.0.0", "localhost", 1),
		config.Server.Port)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// basicAuth wraps a handler with HTTP Basic Authentication
func basicAuth(handler http.HandlerFunc, username, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()

		if !ok || user != username || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Admin Access"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		handler(w, r)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Limit the request body size to prevent DoS attacks
	r.Body = http.MaxBytesReader(w, r.Body, MaxFileSize+1024) // Add a little extra for form fields

	// Parse the multipart form with a reasonable max memory
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Println("Error parsing multipart form:", err)
		sendErrorResponse(w, "Error processing file upload. The file may be too large.", http.StatusBadRequest)
		return
	}

	// Get the file from the form
	file, header, err := r.FormFile("file")
	if err != nil {
		log.Println("Error getting file from form:", err)
		sendErrorResponse(w, "No file uploaded or error reading file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Check file size
	if header.Size > MaxFileSize {
		log.Printf("File too large: %s (%d bytes)", header.Filename, header.Size)
		sendErrorResponse(w, fmt.Sprintf("File too large. Maximum size is %d MB", MaxFileSize/(1024*1024)), http.StatusBadRequest)
		return
	}

	// Validate file extension if restrictions are in place
	if len(allowedExtensions) > 0 {
		fileExt := strings.ToLower(filepath.Ext(header.Filename))
		if !isAllowedExtension(fileExt) {
			log.Printf("Invalid file type: %s", fileExt)
			sendErrorResponse(w, "File type not allowed", http.StatusBadRequest)
			return
		}
	}

	// Get expiration time from form (default to 5 minutes)
	expirationMinutes := DefaultExpiration
	if expStr := r.FormValue("expiration"); expStr != "" {
		if exp, err := strconv.Atoi(expStr); err == nil && exp > 0 {
			expirationMinutes = exp
		}
	}

	// Check if password protection is enabled
	password := r.FormValue("password")
	var passwordHash string
	isProtected := false
	if password != "" {
		// Create a simple hash of the password
		hash := sha256.Sum256([]byte(password))
		passwordHash = hex.EncodeToString(hash[:])
		isProtected = true
	}

	// Create a unique filename to avoid collisions
	uniqueID, err := generateToken()
	if err != nil {
		sendErrorResponse(w, "Failed to generate unique ID", http.StatusInternalServerError)
		return
	}

	// Extract file extension and create a unique filename
	fileExt := filepath.Ext(header.Filename)
	uniqueFilename := uniqueID + fileExt
	dstPath := filepath.Join("uploads", uniqueFilename)

	// Create the destination file
	dst, err := os.Create(dstPath)
	if err != nil {
		log.Println("Error creating destination file:", err)
		sendErrorResponse(w, "Could not save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Stream the file content directly to the disk
	size, err := io.Copy(dst, file)
	if err != nil {
		log.Println("Error saving file content:", err)
		sendErrorResponse(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	// Close the file to ensure all data is written
	dst.Close()

	// Check if we should compress this file
	isCompressed := false
	originalSize := size

	// Get compression preference from form
	doCompress := true
	if compressStr := r.FormValue("compress"); compressStr == "false" {
		doCompress = false
	}

	if doCompress && shouldCompressFile(header.Filename) {
		log.Printf("Compressing file: %s", header.Filename)

		// Compress the file
		compressedPath, err := compressFile(dstPath, header.Filename)
		if err != nil {
			log.Printf("Error compressing file: %v", err)
			// Continue with the original file if compression fails
		} else {
			// Get the size of the compressed file
			compressedInfo, err := os.Stat(compressedPath)
			if err != nil {
				log.Printf("Error getting compressed file info: %v", err)
			} else if compressedInfo.Size() < size {
				// Use the compressed file only if it's smaller
				os.Remove(dstPath) // Remove the original file
				dstPath = compressedPath
				size = compressedInfo.Size()
				isCompressed = true
				log.Printf("File compressed successfully. Original: %d bytes, Compressed: %d bytes (%.1f%%)",
					originalSize, size, float64(size)/float64(originalSize)*100)
			} else {
				// Compressed file is not smaller, remove it and use the original
				os.Remove(compressedPath)
				log.Printf("Compression did not reduce file size. Using original file.")
			}
		}
	}

	log.Printf("Uploaded File: %s (saved as %s), Size: %d bytes, Compressed: %v\n",
		header.Filename, filepath.Base(dstPath), size, isCompressed)

	// Generate a token for the download link
	token, err := generateToken()
	if err != nil {
		sendErrorResponse(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Encrypt the token for URL security
	encryptedToken, err := encryptToken(token)
	if err != nil {
		log.Println("Error encrypting token:", err)
		sendErrorResponse(w, "Failed to secure download link", http.StatusInternalServerError)
		return
	}

	// Calculate expiration time
	now := time.Now()
	expiresAt := now.Add(time.Duration(expirationMinutes) * time.Minute)

	// Create the shareable link with encrypted token
	shareableLink := fmt.Sprintf("http://%s/files/%s", r.Host, encryptedToken)

	// Check if this is an encrypted file
	isEncrypted := false
	originalFilename := header.Filename

	// If the file was encrypted client-side, get the original name
	if encryptedParam := r.FormValue("encrypted"); encryptedParam == "true" {
		isEncrypted = true
		if origName := r.FormValue("originalName"); origName != "" {
			originalFilename = origName
		}
	}

	// Store file information
	fileInfo := &fileInfo{
		Token:        token,
		Path:         dstPath,
		OriginalName: originalFilename,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
		Password:     passwordHash,
		IsProtected:  isProtected,
		Size:         size,
		OriginalSize: originalSize,
		IsCompressed: isCompressed,
		IsEncrypted:  isEncrypted,
		URL:          shareableLink,
	}

	mutex.Lock()
	fileTokens[token] = fileInfo
	mutex.Unlock()

	// Prepare the response
	response := uploadResponse{
		Success:      true,
		URL:          shareableLink,
		ExpiresIn:    expirationMinutes,
		IsProtected:  isProtected,
		OriginalName: originalFilename,
		Size:         size,
		OriginalSize: originalSize,
		IsCompressed: isCompressed,
		IsEncrypted:  isEncrypted,
	}

	// Set content type and send JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// sendErrorResponse sends a standardized JSON error response
func sendErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	response := errorResponse{
		Success: false,
		Message: message,
	}

	json.NewEncoder(w).Encode(response)
}

// isAllowedExtension checks if a file extension is in the allowed list
func isAllowedExtension(ext string) bool {
	ext = strings.ToLower(ext)

	// Special case for encrypted files
	if strings.HasSuffix(ext, ".encrypted") {
		return true
	}

	for _, allowed := range allowedExtensions {
		if ext == allowed {
			return true
		}
	}
	return false
}

// compressFile compresses a file using zip format
func compressFile(srcPath, originalName string) (string, error) {
	// Create a buffer to write our archive to
	buf := new(bytes.Buffer)

	// Create a new zip archive
	zipWriter := zip.NewWriter(buf)

	// Open the source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", err
	}
	defer srcFile.Close()

	// Get file info for header
	info, err := srcFile.Stat()
	if err != nil {
		return "", err
	}

	// Create a header based on the file info
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return "", err
	}

	// Use original filename instead of the unique ID filename
	header.Name = originalName

	// Set compression method
	header.Method = zip.Deflate

	// Create the file in the zip archive
	writer, err := zipWriter.CreateHeader(header)
	if err != nil {
		return "", err
	}

	// Copy the file content to the zip
	_, err = io.Copy(writer, srcFile)
	if err != nil {
		return "", err
	}

	// Close the zip writer
	if err := zipWriter.Close(); err != nil {
		return "", err
	}

	// Create a new file for the compressed data
	compressedPath := srcPath + ".zip"
	compressedFile, err := os.Create(compressedPath)
	if err != nil {
		return "", err
	}
	defer compressedFile.Close()

	// Write the buffer to the file
	_, err = buf.WriteTo(compressedFile)
	if err != nil {
		return "", err
	}

	return compressedPath, nil
}

// shouldCompressFile determines if a file should be compressed based on its extension
func shouldCompressFile(filename string) bool {
	// Don't compress already compressed formats
	noCompressExtensions := []string{
		".zip", ".rar", ".7z", ".gz", ".tar", ".bz2", ".xz",
		".jpg", ".jpeg", ".png", ".gif", ".mp3", ".mp4", ".avi", ".mov",
		".pdf", // PDFs are often already compressed
	}

	ext := strings.ToLower(filepath.Ext(filename))
	for _, noCompressExt := range noCompressExtensions {
		if ext == noCompressExt {
			return false
		}
	}

	return true
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	// Extract encrypted token from URL
	encryptedToken := r.URL.Path[len("/files/"):]

	// Decrypt the token
	token, err := decryptToken(encryptedToken)
	if err != nil {
		log.Printf("Error decrypting token: %v", err)
		serveErrorPage(w, "Invalid or corrupted download link.", http.StatusBadRequest)
		return
	}

	mutex.Lock()
	info, ok := fileTokens[token] // Check if token exists
	if !ok {
		// If not, it's either invalid or was already cleaned up by the background job.
		mutex.Unlock()
		serveErrorPage(w, "Link is invalid or has expired.", http.StatusNotFound)
		return
	}

	// Check if the file has expired
	if time.Now().After(info.ExpiresAt) {
		delete(fileTokens, token)
		mutex.Unlock()
		serveErrorPage(w, "This link has expired.", http.StatusGone)
		return
	}

	// Check if password is required
	if info.Password != "" {
		// If this is not a POST request with password, show password form
		if r.Method != http.MethodPost {
			mutex.Unlock() // Unlock before rendering the form
			servePasswordForm(w, r, token)
			return
		}

		// If this is a POST, verify the password
		if err := r.ParseForm(); err != nil {
			mutex.Unlock()
			serveErrorPage(w, "Error processing password.", http.StatusBadRequest)
			return
		}

		password := r.FormValue("password")
		hash := sha256.Sum256([]byte(password))
		passwordHash := hex.EncodeToString(hash[:])

		if passwordHash != info.Password {
			mutex.Unlock()
			servePasswordForm(w, r, token, "Incorrect password. Please try again.")
			return
		}
	}

	// If we get here, the token is valid and password (if any) is correct
	// For one-time use, delete the token now
	delete(fileTokens, token)
	filePath := info.Path
	originalName := info.OriginalName
	mutex.Unlock()

	// Defer file removal to ensure it's cleaned up after the function returns
	defer os.Remove(filePath)

	// Open the file to be served
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ERROR: could not open file %s: %v", filePath, err)
		serveErrorPage(w, "File not found on server.", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file stats for headers
	fileStat, err := file.Stat()
	if err != nil {
		log.Printf("ERROR: could not stat file %s: %v", filePath, err)
		serveErrorPage(w, "Could not get file information.", http.StatusInternalServerError)
		return
	}

	// Check if this is a raw request (for encrypted files)
	if rawParam := r.URL.Query().Get("raw"); rawParam == "true" {
		// For encrypted files, we want to serve the raw file without forcing download
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, originalName))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileStat.Size()))
	} else {
		// For regular downloads, force download with the correct filename
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, originalName))
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", fileStat.Size()))

		// Check if this is an encrypted file and redirect to the decrypt page
		if strings.HasSuffix(originalName, ".encrypted") {
			http.Redirect(w, r, fmt.Sprintf("/decrypt/%s", encryptedToken), http.StatusSeeOther)
			return
		}
	}

	// Stream the file to the response writer
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("ERROR: failed to write file to response: %v", err)
	}

	log.Printf("Download of %s complete. The file has been deleted.", originalName)
}

// servePasswordForm displays a form to enter password for protected files
func servePasswordForm(w http.ResponseWriter, r *http.Request, token string, errorMsg ...string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	errorHTML := ""
	if len(errorMsg) > 0 && errorMsg[0] != "" {
		errorHTML = fmt.Sprintf(`<div class="error">%s</div>`, errorMsg[0])
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Password Protected File - Share-it</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/water.css@2/out/water.css">
    <style>
        body {
            max-width: 500px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background-color: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 4px 8px rgba(0,0,0,0.1);
            text-align: center;
        }
        .error {
            color: #e74c3c;
            margin-bottom: 15px;
        }
    </style>
</head>
<body>
    <div class="container">
        <h2>Password Protected File</h2>
        <p>This file is protected with a password. Please enter the password to download.</p>
        %s
        <form method="post" action="/files/%s">
            <div>
                <label for="password">Password:</label>
                <input type="password" name="password" id="password" required autofocus>
            </div>
            <button type="submit">Download File</button>
        </form>
    </div>
</body>
</html>`, errorHTML, token)

	w.Write([]byte(html))
}

// serveErrorPage displays a user-friendly error page
func serveErrorPage(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error - Share-it</title>
    <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/water.css@2/out/water.css">
    <style>
        body {
            max-width: 500px;
            margin: 0 auto;
            padding: 20px;
        }
        .container {
            background-color: white;
            padding: 2rem;
            border-radius: 8px;
            box-shadow: 0 4px 8px rgba(0,0,0,0.1);
            text-align: center;
        }
        .error-message {
            color: #e74c3c;
            margin: 20px 0;
        }
    </style>
</head>
<body>
    <div class="container">
        <h2>Error</h2>
        <div class="error-message">%s</div>
        <p><a href="/">Return to homepage</a></p>
    </div>
</body>
</html>`, message)

	w.Write([]byte(html))
}

// statsHandler provides basic statistics about the server
func statsHandler(w http.ResponseWriter, r *http.Request) {
	mutex.Lock()
	defer mutex.Unlock()

	// Count active files and total size
	activeFiles := len(fileTokens)
	var totalSize int64
	for _, info := range fileTokens {
		totalSize += info.Size
	}

	// Prepare response
	stats := struct {
		ActiveFiles int    `json:"activeFiles"`
		TotalSize   int64  `json:"totalSizeBytes"`
		ServerTime  string `json:"serverTime"`
	}{
		ActiveFiles: activeFiles,
		TotalSize:   totalSize,
		ServerTime:  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// filesListHandler returns a list of all active files (admin only)
func filesListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mutex.Lock()
	defer mutex.Unlock()

	// Create a slice to hold all file info objects
	files := make([]*fileInfo, 0, len(fileTokens))

	// Add each file to the slice
	for _, info := range fileTokens {
		files = append(files, info)
	}

	// Send the response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// fileActionHandler handles actions on individual files (admin only)
func fileActionHandler(w http.ResponseWriter, r *http.Request) {
	// Extract token from URL
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		sendErrorResponse(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// For admin actions, we use the raw token without encryption
	token := parts[len(parts)-1]

	// Handle DELETE request to remove a file
	if r.Method == http.MethodDelete {
		mutex.Lock()
		info, exists := fileTokens[token]
		if !exists {
			mutex.Unlock()
			sendErrorResponse(w, "File not found", http.StatusNotFound)
			return
		}

		// Get the file path before deleting from map
		filePath := info.Path
		delete(fileTokens, token)
		mutex.Unlock()

		// Delete the file from disk
		if err := os.Remove(filePath); err != nil {
			log.Printf("Error deleting file %s: %v", filePath, err)
			// Continue anyway, as the token is already removed
		}

		// Send success response
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"message": "File deleted successfully",
		})
		return
	}

	// Handle GET request to get file details
	if r.Method == http.MethodGet {
		mutex.Lock()
		info, exists := fileTokens[token]
		mutex.Unlock()

		if !exists {
			sendErrorResponse(w, "File not found", http.StatusNotFound)
			return
		}

		// Send file info
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
		return
	}

	// If we get here, the method is not supported
	sendErrorResponse(w, "Method not allowed", http.StatusMethodNotAllowed)
}

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
	MaxFileSize = int64(config.Files.MaxSizeMB) * 1024 * 1024
	DefaultExpiration = config.Files.DefaultExpirationMinutes
	AdminUsername = config.Admin.Username
	AdminPassword = config.Admin.Password

	if len(config.Files.AllowedExtensions) > 0 {
		allowedExtensions = config.Files.AllowedExtensions
	}

	log.Println("Configuration loaded successfully")
}

// setDefaultConfig sets default values for the configuration
func setDefaultConfig() {
	config.Server.Port = 8081
	config.Server.Host = "0.0.0.0"
	config.Files.MaxSizeMB = 100
	config.Files.DefaultExpirationMinutes = 5
	config.Files.AllowedExtensions = allowedExtensions
	config.Admin.Username = AdminUsername
	config.Admin.Password = AdminPassword
}

// generateToken creates a secure random token
func generateToken() (string, error) {
	b := make([]byte, 16) // 16 bytes = 32 hex characters
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// encryptToken encrypts a token for URL security
func encryptToken(token string) (string, error) {
	// Create a new AES cipher block
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	// Create a new GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create a nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt the token
	ciphertext := gcm.Seal(nonce, nonce, []byte(token), nil)

	// Encode to base64 for URL safety
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// decryptToken decrypts a token from URL
func decryptToken(encryptedToken string) (string, error) {
	// Decode from base64
	ciphertext, err := base64.URLEncoding.DecodeString(encryptedToken)
	if err != nil {
		return "", err
	}

	// Create a new AES cipher block
	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}

	// Create a new GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Check if the ciphertext is long enough
	if len(ciphertext) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}

	// Extract the nonce and ciphertext
	nonce, ciphertext := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]

	// Decrypt the token
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// cleanupExpiredFiles runs in the background to remove files that have expired.
func cleanupExpiredFiles() {
	// Create a ticker that fires every minute.
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	// Loop indefinitely, waiting for the ticker.
	for range ticker.C {
		mutex.Lock() // Lock the global map to safely iterate over it.

		var pathsToDelete []string
		now := time.Now()
		for token, info := range fileTokens {
			if now.After(info.ExpiresAt) {
				pathsToDelete = append(pathsToDelete, info.Path)
				delete(fileTokens, token) // Delete the token from the map.
			}
		}

		mutex.Unlock() // Unlock before performing slow I/O operations.

		if len(pathsToDelete) > 0 {
			log.Printf("Cleanup: Removing %d expired file(s).", len(pathsToDelete))
			for _, path := range pathsToDelete {
				os.Remove(path) // Remove the actual file from disk.
			}
		}
	}
}
