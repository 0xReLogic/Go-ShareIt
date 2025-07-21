package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Global variables for state management
var (
	fileTokens = make(map[string]fileInfo)
	mutex      = &sync.Mutex{}
)

// fileInfo stores the path and creation time for a token
type fileInfo struct {
	Path      string
	CreatedAt time.Time
	mu        sync.RWMutex
}

func main() {
	// Setup handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, "index.html")
	})
	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/files/", downloadHandler)

	// Create uploads directory if it doesn't exist
	if err := os.MkdirAll("uploads", os.ModePerm); err != nil {
		log.Fatal("Failed to create uploads directory:", err)
	}

	log.Println("Server starting on http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use MultipartReader to handle large files by streaming
	reader, err := r.MultipartReader()
	if err != nil {
		log.Println("Error creating multipart reader:", err)
		http.Error(w, "Error processing file upload", http.StatusInternalServerError)
		return
	}

	part, err := reader.NextPart()
	if err == io.EOF { // No file part
		http.Error(w, "No file uploaded", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Println("Error reading multipart data:", err)
		http.Error(w, "Error processing file upload", http.StatusInternalServerError)
		return
	}
	defer part.Close()

	if part.FormName() != "file" {
		http.Error(w, "Invalid form field name. Expected 'file'", http.StatusBadRequest)
		return
	}

	filename := part.FileName()
	if filename == "" {
		http.Error(w, "Filename is empty", http.StatusBadRequest)
		return
	}

	dstPath := filepath.Join("uploads", filename)
	dst, err := os.Create(dstPath)
	if err != nil {
		log.Println("Error creating destination file:", err)
		http.Error(w, "Could not save file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Stream the file content directly to the disk
	size, err := io.Copy(dst, part)
	if err != nil {
		log.Println("Error saving file content:", err)
		http.Error(w, "Error saving file", http.StatusInternalServerError)
		return
	}

	log.Printf("Uploaded File: %s, Size: %d bytes\n", filename, size)

	token, err := generateToken()
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	mutex.Lock()
	fileTokens[token] = fileInfo{Path: dstPath, CreatedAt: time.Now()}
	mutex.Unlock()

	shareableLink := fmt.Sprintf("http://%s/files/%s", r.Host, token)
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Upload Successful</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/water.css@2/out/water.css"></head><body><h2>File Uploaded Successfully!</h2><p>Shareable link: <a href="%s">%s</a></p><p>This link will expire in 5 minutes and is valid for one download only.</p><p><a href="/">Upload another file</a></p></body></html>`, shareableLink, shareableLink)
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Path[len("/files/"):]

	mutex.Lock()
	info, ok := fileTokens[token]

	// Check if token is valid
	if !ok {
		mutex.Unlock()
		http.Error(w, "Link is invalid or has expired.", http.StatusNotFound)
		return
	}

	// Lock the file-specific mutex to prevent other downloads of the same file
	info.mu.Lock()
	defer info.mu.Unlock()

	// Check if the file has expired
	if time.Since(info.CreatedAt) > time.Minute*5 {
		delete(fileTokens, token)
		os.Remove(info.Path)
		mutex.Unlock()
		http.Error(w, "Link is invalid or has expired.", http.StatusNotFound)
		return
	}

	// If token is valid, delete it immediately to make it a one-time use token.
	delete(fileTokens, token)
	mutex.Unlock()

	// Defer file removal to ensure it's cleaned up after the function returns
	defer os.Remove(info.Path)

	// Open the file to be served
	file, err := os.Open(info.Path)
	if err != nil {
		log.Printf("ERROR: could not open file %s: %v", info.Path, err)
		http.Error(w, "File not found on server.", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Get file stats for headers
	fileStat, err := file.Stat()
	if err != nil {
		log.Printf("ERROR: could not stat file %s: %v", info.Path, err)
		http.Error(w, "Could not get file information.", http.StatusInternalServerError)
		return
	}

	// Set headers to force download with correct filename
	w.Header().Set("Content-Disposition", "attachment; filename=\""+filepath.Base(info.Path)+"\"")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileStat.Size()))

	// Stream the file to the response writer
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("ERROR: failed to write file to response: %v", err)
	}

	log.Printf("Download of %s complete. The file has been deleted.", filepath.Base(info.Path))
}

func generateToken() (string, error) {
	b := make([]byte, 16) // Increased token length for better security
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
