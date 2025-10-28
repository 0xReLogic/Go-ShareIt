package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Response structure for upload API
type UploadResponse struct {
	Success      bool   `json:"success"`
	URL          string `json:"url"`
	ExpiresIn    int    `json:"expiresIn"`
	IsProtected  bool   `json:"isProtected"`
	OriginalName string `json:"originalName"`
	Size         int64  `json:"size"`
	OriginalSize int64  `json:"originalSize"`
	IsCompressed bool   `json:"isCompressed"`
}

// Error response structure
type ErrorResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

func main() {
	// Define command-line flags
	serverURL := flag.String("server", "http://localhost:8080", "Server URL")
	expiration := flag.Int("expire", 5, "Expiration time in minutes (5, 15, 60, 1440)")
	password := flag.String("password", "", "Password protection (optional)")
	compress := flag.Bool("compress", true, "Enable automatic compression")

	// Parse flags
	flag.Parse()

	// Get the file path from remaining arguments
	args := flag.Args()
	if len(args) == 0 {
		fmt.Println("Error: No file specified")
		fmt.Println("Usage: share-it-cli [options] <file>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	filePath := args[0]

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	if fileInfo.IsDir() {
		fmt.Println("Error: Cannot upload directories")
		os.Exit(1)
	}

	// Upload the file
	fmt.Printf("Uploading %s...\n", filePath)
	response, err := uploadFile(filePath, *serverURL, *expiration, *password, *compress)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	// Print the result
	fmt.Println("\nUpload successful!")
	fmt.Printf("Shareable link: %s\n", response.URL)
	fmt.Printf("Expires in: %d minutes\n", response.ExpiresIn)

	if response.IsProtected {
		fmt.Println("Password protected: Yes")
	} else {
		fmt.Println("Password protected: No")
	}

	if response.IsCompressed {
		originalSize := formatFileSize(response.OriginalSize)
		compressedSize := formatFileSize(response.Size)
		savingPercent := (1 - float64(response.Size)/float64(response.OriginalSize)) * 100
		fmt.Printf("Compression: Enabled (saved %.1f%%, from %s to %s)\n",
			savingPercent, originalSize, compressedSize)
	} else {
		fmt.Printf("Size: %s\n", formatFileSize(response.Size))
	}

	fmt.Println("\nNote: This link is valid for one download only and will expire after the specified time.")
}

// uploadFile uploads a file to the server
func uploadFile(filePath, serverURL string, expiration int, password string, compress bool) (*UploadResponse, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %v", err)
	}
	defer file.Close()

	// Create multipart form
	requestBody, contentType, err := createMultipartForm(file, filePath, expiration, password, compress)
	if err != nil {
		return nil, err
	}

	// Send the request
	uploadURL := fmt.Sprintf("%s/upload", serverURL)
	resp, err := sendUploadRequest(uploadURL, requestBody, contentType)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Parse the response
	return parseUploadResponse(resp)
}

// createMultipartForm creates the multipart form data for file upload
func createMultipartForm(file *os.File, filePath string, expiration int, password string, compress bool) (*bytes.Buffer, string, error) {
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)

	// Create a form file field
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return nil, "", fmt.Errorf("failed to create form file: %v", err)
	}

	// Copy the file content to the form field
	if _, err := io.Copy(part, file); err != nil {
		return nil, "", fmt.Errorf("failed to copy file content: %v", err)
	}

	// Add form fields
	if err := addFormFields(writer, expiration, password, compress); err != nil {
		return nil, "", err
	}

	// Close the writer
	if err := writer.Close(); err != nil {
		return nil, "", fmt.Errorf("failed to close writer: %v", err)
	}

	return &requestBody, writer.FormDataContentType(), nil
}

// addFormFields adds the necessary form fields to the multipart writer
func addFormFields(writer *multipart.Writer, expiration int, password string, compress bool) error {
	if err := writer.WriteField("expiration", fmt.Sprintf("%d", expiration)); err != nil {
		return fmt.Errorf("failed to add expiration field: %v", err)
	}

	if password != "" {
		if err := writer.WriteField("password", password); err != nil {
			return fmt.Errorf("failed to add password field: %v", err)
		}
	}

	if err := writer.WriteField("compress", fmt.Sprintf("%t", compress)); err != nil {
		return fmt.Errorf("failed to add compress field: %v", err)
	}

	return nil
}

// sendUploadRequest sends the HTTP request to upload the file
func sendUploadRequest(uploadURL string, requestBody *bytes.Buffer, contentType string) (*http.Response, error) {
	req, err := http.NewRequest("POST", uploadURL, requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	req.Header.Set("Content-Type", contentType)

	client := &http.Client{
		Timeout: 5 * time.Minute,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}

	return resp, nil
}

// parseUploadResponse parses the server response
func parseUploadResponse(resp *http.Response) (*UploadResponse, error) {
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errorResp ErrorResponse
		if err := json.Unmarshal(body, &errorResp); err == nil {
			return nil, fmt.Errorf("server error: %s", errorResp.Message)
		}
		return nil, fmt.Errorf("server error: %s", resp.Status)
	}

	var uploadResp UploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	return &uploadResp, nil
}

// formatFileSize formats a file size in bytes to a human-readable string
func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
