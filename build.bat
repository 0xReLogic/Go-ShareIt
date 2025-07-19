@echo off
echo Creating build directory...
if not exist releases mkdir releases

echo Building for Windows (amd64)...
set GOOS=windows
set GOARCH=amd64
go build -ldflags="-s -w" -o releases/share-it-windows-amd64.exe .

echo Building for macOS (amd64)...
set GOOS=darwin
set GOARCH=amd64
go build -ldflags="-s -w" -o releases/share-it-macos-amd64 .

echo Building for macOS (arm64)...
set GOOS=darwin
set GOARCH=arm64
go build -ldflags="-s -w" -o releases/share-it-macos-arm64 .

echo Building for Linux (amd64)...
set GOOS=linux
set GOARCH=amd64
go build -ldflags="-s -w" -o releases/share-it-linux-amd64 .

echo Building for Linux (arm64)...
set GOOS=linux
set GOARCH=arm64
go build -ldflags="-s -w" -o releases/share-it-linux-arm64 .

echo Build complete! Check the 'releases' directory.
