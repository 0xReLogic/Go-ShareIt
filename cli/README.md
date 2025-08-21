# Share-it CLI

A command-line interface for the Share-it file sharing application.

## Features

- Upload files directly from the command line
- Set custom expiration times
- Add password protection
- Enable/disable automatic compression
- Get shareable links instantly

## Usage

```
share-it-cli [options] <file>
```

### Options

- `-server string`: Server URL (default "http://localhost:8080")
- `-expire int`: Expiration time in minutes (5, 15, 60, 1440) (default 5)
- `-password string`: Password protection (optional)
- `-compress`: Enable automatic compression (default true)

### Examples

Upload a file with default settings:
```
share-it-cli document.pdf
```

Upload a file with custom expiration (1 hour):
```
share-it-cli -expire 60 document.pdf
```

Upload a file with password protection:
```
share-it-cli -password mysecretpassword document.pdf
```

Upload a file to a custom server:
```
share-it-cli -server http://192.168.1.100:8080 document.pdf
```

Upload a file without compression:
```
share-it-cli -compress=false large-archive.zip
```

## Building from Source

### Windows

```
build.bat
```

### Linux/macOS

```
chmod +x build.sh
./build.sh
```

The compiled binaries will be placed in the `releases` directory.

## Requirements

- Go 1.16 or higher (for building from source)
- Running Share-it server