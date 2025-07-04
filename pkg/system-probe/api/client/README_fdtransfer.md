# File Descriptor Transfer Module

This module provides functionality to transfer file descriptors between the core agent and system-probe over Unix sockets using `SCM_RIGHTS`. This is particularly useful when the core agent needs to access files that require elevated privileges that only system-probe has.

## Overview

The file descriptor transfer module consists of:

1. **System-probe module** (`cmd/system-probe/modules/fdtransfer.go`): Handles requests to open files and transfer file descriptors
2. **Client package** (`pkg/system-probe/api/client/fdtransfer.go`): Provides functions for the core agent to request file descriptors

## How it works

1. The core agent sends an HTTP POST request to system-probe with the file path
2. System-probe opens the file and sends the file descriptor via `SCM_RIGHTS` over the Unix socket
3. The core agent receives the file descriptor and can use it to access the file

## Usage

### From the core agent

```go
import (
    "os"
    "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
)

// Open a file and get its file descriptor
socketPath := "/opt/datadog-agent/run/sysprobe.sock" // Default system-probe socket path
filePath := "/etc/hosts" // Example file to open

file, err := client.OpenFile(socketPath, filePath)
if err != nil {
    log.Errorf("Failed to open file: %v", err)
    return
}
defer file.Close()

// Now you can use the file descriptor
// For example, read from it:
data := make([]byte, 1024)
n, err := file.Read(data)
if err != nil {
    log.Errorf("Failed to read from file: %v", err)
    return
}
log.Infof("Read %d bytes: %s", n, string(data[:n]))
```

## Security Considerations

- Only absolute paths are allowed (relative paths are rejected)
- The module only opens files in read-only mode
- File descriptors are properly closed when the connection is closed

## Configuration

The FD transfer module is automatically enabled on Linux systems when system-probe is running. No additional configuration is required.

## API Endpoints

- `POST /fd_transfer/open`: Opens a file and transfers its file descriptor

### Request Format

```json
{
    "path": "/path/to/file"
}
```

### Response Format

```json
{
    "success": true
}
```

Or in case of error:

```json
{
    "success": false,
    "error": "Error message"
}
```

## Limitations

- Only works on Linux systems (uses Unix domain sockets and `SCM_RIGHTS`)
- Files are opened in read-only mode
- Only one file descriptor can be transferred per request
- The file descriptor is tied to the Unix socket connection

## Example Use Cases

- Reading system files that require elevated privileges
- Accessing files in containers from the host
- Reading files that are only accessible to system-probe due to security policies
