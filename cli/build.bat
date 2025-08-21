@echo off
echo Building Share-it CLI...

REM Create releases directory if it doesn't exist
if not exist "releases" mkdir releases

REM Build for Windows (amd64)
echo Building for Windows (amd64)...
go build -o releases/share-it-cli-windows-amd64.exe main.go

REM Build for Linux (amd64)
echo Building for Linux (amd64)...
set GOOS=linux
set GOARCH=amd64
go build -o releases/share-it-cli-linux-amd64 main.go

REM Build for macOS (amd64)
echo Building for macOS (amd64)...
set GOOS=darwin
set GOARCH=amd64
go build -o releases/share-it-cli-darwin-amd64 main.go

REM Reset environment variables
set GOOS=
set GOARCH=

echo Build complete! Binaries are in the 'releases' directory.