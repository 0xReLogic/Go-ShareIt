package main

import "time"

// Constants for repeated string literals
const (
	encryptedExt      = ".encrypted"
	defaultHost       = "0.0.0.0"
	methodNotAllowed  = "Method not allowed"
	contentTypeHeader = "Content-Type"
	contentTypeJSON   = "application/json"
	contentTypeHTML   = "text/html; charset=utf-8"
	contentTypeOctet  = "application/octet-stream"
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

// uploadParams holds parameters extracted from the upload request
type uploadParams struct {
	expirationMinutes int
	passwordHash      string
	isProtected       bool
	doCompress        bool
	isEncrypted       bool
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
