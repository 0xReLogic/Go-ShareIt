package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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

	log.Printf("File uploaded successfully (saved as %s), Size: %d bytes, Compressed: %v\n",
		filepath.Base(finalPath), finalSize, isCompressed)

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
		log.Printf("File too large: %d bytes", header.Size)
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
		params.passwordHash = hashPassword(password)
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

	log.Println("Compressing file...")

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
