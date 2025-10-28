package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

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
