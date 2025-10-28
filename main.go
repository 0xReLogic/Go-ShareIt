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

// Constants for repeated string literals
const (
	encryptedExt       = ".encrypted"
	defaultHost        = "0.0.0.0"
	methodNotAllowed   = "Method not allowed"
	contentTypeHeader  = "Content-Type"
	contentTypeJSON    = "application/json"
	contentTypeHTML    = "text/html; charset=utf-8"
	contentTypeOctet   = "application/octet-stream"
)

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
		".7z", ".mp3", ".mp4", ".avi", ".mov", encryptedExt,
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
		strings.Replace(config.Server.Host, defaultHost, "localhost", 1),
		config.Server.Port)
	log.Printf("Admin dashboard available at http://%s:%d/admin",
		strings.Replace(config.Server.Host, defaultHost, "localhost", 1),
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
		http.Error(w, methodNotAllowed, http.StatusMethodNotAllowed)
		return
	}

	// Parse and validate the upload request
	file, header, err := parseUploadRequest(r)
	if err != nil {
		sendErrorResponse(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get upload parameters
	params := extractUploadParams(r)

	// Save the uploaded file
	dstPath, size, err := saveUploadedFile(file, header)
	if err != nil {
		sendErrorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Handle compression if needed
	finalPath, finalSize, isCompressed := handleFileCompression(dstPath, header.Filename, size, params.doCompress)

	log.Printf("Uploaded File: %s (saved as %s), Size: %d bytes, Compressed: %v\n",
		header.Filename, filepath.Base(finalPath), finalSize, isCompressed)

	// Generate shareable link
	shareableLink, token, err := generateShareableLink(r.Host)
	if err != nil {
		sendErrorResponse(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Store file information
	originalFilename := getOriginalFilename(header.Filename, params.isEncrypted, r.FormValue("originalName"))
	storeFileInfo(fileStorageInfo{
		token:        token,
		path:         finalPath,
		originalName: originalFilename,
		params:       params,
		size:         finalSize,
		originalSize: size,
		isCompressed: isCompressed,
		url:          shareableLink,
	})

	// Send response
	sendUploadResponse(w, shareableLink, params, originalFilename, finalSize, size, isCompressed)
}

// uploadParams holds parameters extracted from the upload request
type uploadParams struct {
	expirationMinutes int
	passwordHash      string
	isProtected       bool
	doCompress        bool
	isEncrypted       bool
}

// parseUploadRequest parses and validates the multipart form upload
func parseUploadRequest(r *http.Request) (multipart.File, *multipart.FileHeader, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, MaxFileSize+1024)

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		log.Println("Error parsing multipart form:", err)
		return nil, nil, errors.New("error processing file upload. The file may be too large")
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		log.Println("Error getting file from form:", err)
		return nil, nil, errors.New("no file uploaded or error reading file")
	}

	if header.Size > MaxFileSize {
		log.Printf("File too large: %s (%d bytes)", header.Filename, header.Size)
		return nil, nil, fmt.Errorf("file too large. Maximum size is %d MB", MaxFileSize/(1024*1024))
	}

	if len(allowedExtensions) > 0 {
		fileExt := strings.ToLower(filepath.Ext(header.Filename))
		if !isAllowedExtension(fileExt) {
			log.Printf("Invalid file type: %s", fileExt)
			return nil, nil, errors.New("file type not allowed")
		}
	}

	return file, header, nil
}

// extractUploadParams extracts upload parameters from the request
func extractUploadParams(r *http.Request) uploadParams {
	params := uploadParams{
		expirationMinutes: DefaultExpiration,
		doCompress:        true,
	}

	if expStr := r.FormValue("expiration"); expStr != "" {
		if exp, err := strconv.Atoi(expStr); err == nil && exp > 0 {
			params.expirationMinutes = exp
		}
	}

	if password := r.FormValue("password"); password != "" {
		hash := sha256.Sum256([]byte(password))
		params.passwordHash = hex.EncodeToString(hash[:])
		params.isProtected = true
	}

	if compressStr := r.FormValue("compress"); compressStr == "false" {
		params.doCompress = false
	}

	if encryptedParam := r.FormValue("encrypted"); encryptedParam == "true" {
		params.isEncrypted = true
	}

	return params
}

// saveUploadedFile saves the uploaded file to disk
func saveUploadedFile(file multipart.File, header *multipart.FileHeader) (string, int64, error) {
	uniqueID, err := generateToken()
	if err != nil {
		return "", 0, errors.New("failed to generate unique ID")
	}

	fileExt := filepath.Ext(header.Filename)
	uniqueFilename := uniqueID + fileExt
	dstPath := filepath.Join("uploads", uniqueFilename)

	dst, err := os.Create(dstPath)
	if err != nil {
		log.Println("Error creating destination file:", err)
		return "", 0, errors.New("could not save file")
	}
	defer dst.Close()

	size, err := io.Copy(dst, file)
	if err != nil {
		log.Println("Error saving file content:", err)
		return "", 0, errors.New("error saving file")
	}

	return dstPath, size, nil
}

// handleFileCompression compresses the file if needed
func handleFileCompression(dstPath, filename string, size int64, doCompress bool) (string, int64, bool) {
	if !doCompress || !shouldCompressFile(filename) {
		return dstPath, size, false
	}

	log.Printf("Compressing file: %s", filename)

	compressedPath, err := compressFile(dstPath, filename)
	if err != nil {
		log.Printf("Error compressing file: %v", err)
		return dstPath, size, false
	}

	compressedInfo, err := os.Stat(compressedPath)
	if err != nil {
		log.Printf("Error getting compressed file info: %v", err)
		os.Remove(compressedPath)
		return dstPath, size, false
	}

	if compressedInfo.Size() < size {
		os.Remove(dstPath)
		log.Printf("File compressed successfully. Original: %d bytes, Compressed: %d bytes (%.1f%%)",
			size, compressedInfo.Size(), float64(compressedInfo.Size())/float64(size)*100)
		return compressedPath, compressedInfo.Size(), true
	}

	os.Remove(compressedPath)
	log.Printf("Compression did not reduce file size. Using original file.")
	return dstPath, size, false
}

// generateShareableLink generates an encrypted token and shareable link
func generateShareableLink(host string) (string, string, error) {
	token, err := generateToken()
	if err != nil {
		return "", "", errors.New("failed to generate token")
	}

	encryptedToken, err := encryptToken(token)
	if err != nil {
		log.Println("Error encrypting token:", err)
		return "", "", errors.New("failed to secure download link")
	}

	shareableLink := fmt.Sprintf("http://%s/files/%s", host, encryptedToken)
	return shareableLink, token, nil
}

// getOriginalFilename returns the original filename for the file
func getOriginalFilename(headerFilename string, isEncrypted bool, originalNameParam string) string {
	if isEncrypted && originalNameParam != "" {
		return originalNameParam
	}
	return headerFilename
}

// fileStorageInfo holds all information needed to store a file
type fileStorageInfo struct {
	token        string
	path         string
	originalName string
	params       uploadParams
	size         int64
	originalSize int64
	isCompressed bool
	url          string
}

// storeFileInfo stores the file information in the global map
func storeFileInfo(info fileStorageInfo) {
	now := time.Now()
	expiresAt := now.Add(time.Duration(info.params.expirationMinutes) * time.Minute)

	fileData := &fileInfo{
		Token:        info.token,
		Path:         info.path,
		OriginalName: info.originalName,
		CreatedAt:    now,
		ExpiresAt:    expiresAt,
		Password:     info.params.passwordHash,
		IsProtected:  info.params.isProtected,
		Size:         info.size,
		OriginalSize: info.originalSize,
		IsCompressed: info.isCompressed,
		IsEncrypted:  info.params.isEncrypted,
		URL:          info.url,
	}

	mutex.Lock()
	fileTokens[info.token] = fileData
	mutex.Unlock()
}

// sendUploadResponse sends the JSON response for successful upload
func sendUploadResponse(w http.ResponseWriter, url string, params uploadParams, originalName string, size, originalSize int64, isCompressed bool) {
	response := uploadResponse{
		Success:      true,
		URL:          url,
		ExpiresIn:    params.expirationMinutes,
		IsProtected:  params.isProtected,
		OriginalName: originalName,
		Size:         size,
		OriginalSize: originalSize,
		IsCompressed: isCompressed,
		IsEncrypted:  params.isEncrypted,
	}

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	json.NewEncoder(w).Encode(response)
}

// sendErrorResponse sends a standardized JSON error response
func sendErrorResponse(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set(contentTypeHeader, contentTypeJSON)
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
	if strings.HasSuffix(ext, encryptedExt) {
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
	encryptedToken := r.URL.Path[len("/files/"):]

	token, err := decryptToken(encryptedToken)
	if err != nil {
		log.Printf("Error decrypting token: %v", err)
		serveErrorPage(w, "Invalid or corrupted download link.", http.StatusBadRequest)
		return
	}

	info, filePath, originalName, err := validateAndGetFileInfo(token, r, w)
	if err != nil {
		return // Error already handled
	}

	// Defer file removal to ensure it's cleaned up after the function returns
	defer os.Remove(filePath)

	// Serve the file
	serveFile(w, r, filePath, originalName, encryptedToken)

	log.Printf("Download of %s complete. The file has been deleted.", originalName)
}

// validateAndGetFileInfo validates the token and handles password protection
func validateAndGetFileInfo(token string, r *http.Request, w http.ResponseWriter) (*fileInfo, string, string, error) {
	mutex.Lock()
	info, ok := fileTokens[token]
	if !ok {
		mutex.Unlock()
		serveErrorPage(w, "Link is invalid or has expired.", http.StatusNotFound)
		return nil, "", "", errors.New("token not found")
	}

	if time.Now().After(info.ExpiresAt) {
		delete(fileTokens, token)
		mutex.Unlock()
		serveErrorPage(w, "This link has expired.", http.StatusGone)
		return nil, "", "", errors.New("token expired")
	}

	if info.Password != "" {
		if r.Method != http.MethodPost {
			mutex.Unlock()
			servePasswordForm(w, r, token)
			return nil, "", "", errors.New("password required")
		}

		if err := verifyPassword(r, info.Password); err != nil {
			mutex.Unlock()
			servePasswordForm(w, r, token, "Incorrect password. Please try again.")
			return nil, "", "", err
		}
	}

	delete(fileTokens, token)
	filePath := info.Path
	originalName := info.OriginalName
	mutex.Unlock()

	return info, filePath, originalName, nil
}

// verifyPassword verifies the password from the request
func verifyPassword(r *http.Request, expectedHash string) error {
	if err := r.ParseForm(); err != nil {
		return errors.New("error processing password")
	}

	password := r.FormValue("password")
	hash := sha256.Sum256([]byte(password))
	passwordHash := hex.EncodeToString(hash[:])

	if passwordHash != expectedHash {
		return errors.New("incorrect password")
	}

	return nil
}

// serveFile serves the file to the client
func serveFile(w http.ResponseWriter, r *http.Request, filePath, originalName, encryptedToken string) {
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("ERROR: could not open file %s: %v", filePath, err)
		serveErrorPage(w, "File not found on server.", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	fileStat, err := file.Stat()
	if err != nil {
		log.Printf("ERROR: could not stat file %s: %v", filePath, err)
		serveErrorPage(w, "Could not get file information.", http.StatusInternalServerError)
		return
	}

	setDownloadHeaders(w, originalName, fileStat.Size())

	// Check if this is an encrypted file and redirect to the decrypt page
	if rawParam := r.URL.Query().Get("raw"); rawParam != "true" && strings.HasSuffix(originalName, encryptedExt) {
		http.Redirect(w, r, fmt.Sprintf("/decrypt/%s", encryptedToken), http.StatusSeeOther)
		return
	}

	if _, err := io.Copy(w, file); err != nil {
		log.Printf("ERROR: failed to write file to response: %v", err)
	}
}

// setDownloadHeaders sets the appropriate headers for file download
func setDownloadHeaders(w http.ResponseWriter, filename string, size int64) {
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set(contentTypeHeader, contentTypeOctet)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
}

// servePasswordForm displays a form to enter password for protected files
func servePasswordForm(w http.ResponseWriter, r *http.Request, token string, errorMsg ...string) {
	w.Header().Set(contentTypeHeader, contentTypeHTML)

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
	w.Header().Set(contentTypeHeader, contentTypeHTML)
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

	w.Header().Set(contentTypeHeader, contentTypeJSON)
	json.NewEncoder(w).Encode(stats)
}

// filesListHandler returns a list of all active files (admin only)
func filesListHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		sendErrorResponse(w, methodNotAllowed, http.StatusMethodNotAllowed)
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
	w.Header().Set(contentTypeHeader, contentTypeJSON)
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
		w.Header().Set(contentTypeHeader, contentTypeJSON)
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
		w.Header().Set(contentTypeHeader, contentTypeJSON)
		json.NewEncoder(w).Encode(info)
		return
	}

	// If we get here, the method is not supported
	sendErrorResponse(w, methodNotAllowed, http.StatusMethodNotAllowed)
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
	config.Server.Host = defaultHost
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
