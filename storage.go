package main

import (
	"sync"
	"time"
)

// Global variables for state management
var (
	fileTokens = make(map[string]*fileInfo)
	mutex      = &sync.Mutex{}
)

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
