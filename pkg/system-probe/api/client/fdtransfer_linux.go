// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OpenFileRequest represents a request to open a file and transfer its file descriptor
type OpenFileRequest struct {
	Path string `json:"path"`
}

// OpenFileResponse represents the response from the file descriptor transfer
type OpenFileResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// OpenFileAndGetFD opens a file in system-probe and returns the file descriptor
// This function uses a custom HTTP client that can handle file descriptor transfer
func OpenFileAndGetFD(socketPath string, filePath string) (*os.File, error) {
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system-probe: %v", err)
	}
	defer conn.Close()

	// Create the request
	req := OpenFileRequest{
		Path: filePath,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create HTTP request
	httpReq, err := http.NewRequest("POST", "http://sysprobe/fd_transfer/open", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Write the request to the connection
	if err := httpReq.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request: %v", err)
	}

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("not a Unix connection")
	}

	// Read the message and file descriptor using ReadMsgUnix
	// The server sends the JSON response along with the file descriptor
	buf := make([]byte, 1024) // Larger buffer for JSON response
	oob := make([]byte, syscall.CmsgSpace(4))

	n, oobn, _, _, err := unixConn.ReadMsgUnix(buf, oob)
	if err != nil {
		return nil, fmt.Errorf("ReadMsgUnix failed: %v", err)
	}

	if n == 0 {
		return nil, fmt.Errorf("no response received")
	}

	// Parse the JSON response
	var response OpenFileResponse
	if err := json.Unmarshal(buf[:n], &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("file descriptor transfer failed: %s", response.Error)
	}

	// Parse the file descriptor from the control message
	if oobn > 0 {
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return nil, fmt.Errorf("ParseSocketControlMessage failed: %v", err)
		}

		for _, msg := range msgs {
			if msg.Header.Level == syscall.SOL_SOCKET && msg.Header.Type == syscall.SCM_RIGHTS {
				fds, err := syscall.ParseUnixRights(&msg)
				if err != nil {
					return nil, fmt.Errorf("ParseUnixRights failed: %v", err)
				}

				if len(fds) > 0 {
					fd := fds[0] // We only expect one file descriptor
					log.Debugf("Received file descriptor: %d", fd)
					return os.NewFile(uintptr(fd), filePath), nil
				}
			}
		}
	}

	return nil, fmt.Errorf("no file descriptor received")
}

// OpenFileWithConfig opens a file using the FD transfer module and returns the file descriptor
// This function gets the socket path from the system probe configuration
func OpenFileWithConfig(filePath string) (*os.File, error) {
	socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
	return OpenFileAndGetFD(socketPath, filePath)
}

// OpenFile opens a file using the FD transfer module and returns the file descriptor
func OpenFile(socketPath string, filePath string) (*os.File, error) {
	return OpenFileAndGetFD(socketPath, filePath)
}
