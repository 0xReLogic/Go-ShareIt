package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
