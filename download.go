package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	encryptedToken := r.URL.Path[len("/files/"):]

	token, err := decryptToken(encryptedToken)
	if err != nil {
		log.Printf("Error decrypting token: %v", err)
		serveErrorPage(w, "Invalid or corrupted download link.", http.StatusBadRequest)
		return
	}

	_, filePath, originalName, err := validateAndGetFileInfo(token, r, w)
	if err != nil {
		return // Error already handled
	}

	// Defer file removal to ensure it's cleaned up after the function returns
	defer os.Remove(filePath)

	// Serve the file
	serveFile(w, r, filePath, originalName, encryptedToken)

	log.Println("File download complete. The file has been deleted.")
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
	passwordHash := hashPassword(password)

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
