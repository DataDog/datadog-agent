// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package modules

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(FDTransfer) }

// FDTransfer is a module that provides file descriptor transfer capabilities
var FDTransfer = &module.Factory{
	Name:             config.FDTransferModule,
	ConfigNamespaces: []string{},
	Fn: func(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		log.Infof("Starting the FD Transfer module")
		return &fdTransferModule{}, nil
	},
	NeedsEBPF: func() bool {
		return false
	},
}

var _ module.Module = &fdTransferModule{}

type fdTransferModule struct {
	lastCheck atomic.Int64
}

// GetStats returns stats for the module
func (f *fdTransferModule) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"last_check": f.lastCheck.Load(),
	}
}

// Register registers endpoints for the module to expose data
func (f *fdTransferModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/open", f.openFileHandler).Methods("POST")
	return nil
}

// Close cleans up the module
func (f *fdTransferModule) Close() {
	// No cleanup needed
}

type openFileRequest struct {
	Path string `json:"path"`
}

type openFileResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error,omitempty"`
}

// isLogFile checks if the given path ends with .log
func isLogFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".log")
}

// openFileHandler handles requests to open a file and transfer its file descriptor
func (f *fdTransferModule) openFileHandler(w http.ResponseWriter, r *http.Request) {
	f.lastCheck.Store(time.Now().Unix())

	// read the body here and then hijack the connection
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read body: %v", err)
		return
	}
	log.Infof("Body: %s", string(body))

	// hijack the connection using the Hijacker interface
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		log.Errorf("Could not get hijacker")
		return
	}

	conn, _, err := hijacker.Hijack()
	if err != nil {
		log.Errorf("Failed to hijack connection: %v", err)
		return
	}
	defer conn.Close()

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		log.Errorf("Not a Unix connection")
		response := openFileResponse{
			Success: false,
			Error:   "Not a Unix connection",
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	// Parse the request
	var req openFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		log.Errorf("Failed to parse request: %v", err)
		response := openFileResponse{
			Success: false,
			Error:   "Invalid request format",
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	if req.Path == "" {
		log.Errorf("Empty file path provided")
		response := openFileResponse{
			Success: false,
			Error:   "File path is required",
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	// Validate the path (basic security check)
	if !filepath.IsAbs(req.Path) {
		log.Errorf("Relative path not allowed: %s", req.Path)
		response := openFileResponse{
			Success: false,
			Error:   "Only absolute paths are allowed",
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	// Fully resolve the path (expand symlinks) before validation
	resolvedPath, err := filepath.EvalSymlinks(req.Path)
	if err != nil {
		log.Errorf("Failed to resolve symlinks for %s: %v", req.Path, err)
		response := openFileResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to resolve path: %v", err),
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	// Validate that the resolved file is a log file
	if !isLogFile(resolvedPath) {
		log.Errorf("Non-log file not allowed: %s (resolved from %s)", resolvedPath, req.Path)
		response := openFileResponse{
			Success: false,
			Error:   "Only log files (ending with .log) are allowed",
		}
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	// Open the resolved file with O_NOFOLLOW to prevent symlink attacks
	file, err := os.OpenFile(resolvedPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		log.Errorf("Failed to open file %s (resolved from %s): %v", resolvedPath, req.Path, err)
		response := openFileResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to open file: %v", err),
		}
		// Send error response using WriteMsgUnix (no file descriptor)
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}
	defer file.Close()

	// Get the file descriptor
	fd := int(file.Fd())
	log.Debugf("Sending file descriptor %d for file %s (resolved from %s)", fd, resolvedPath, req.Path)

	// Create success response
	response := openFileResponse{
		Success: true,
	}

	// Send the file descriptor along with the JSON response using WriteMsgUnix
	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Failed to marshal response: %v", err)
		errorResponse := openFileResponse{
			Success: false,
			Error:   "Failed to marshal response",
		}
		errorBytes, _ := json.Marshal(errorResponse)
		if _, _, err := unixConn.WriteMsgUnix(errorBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	rights := syscall.UnixRights(fd)
	_, _, err = unixConn.WriteMsgUnix(responseBytes, rights, nil)
	if err != nil {
		log.Errorf("WriteMsgUnix failed: %v", err)
		response := openFileResponse{
			Success: false,
			Error:   fmt.Sprintf("Failed to send file descriptor: %v", err),
		}
		// Send error response using WriteMsgUnix (no file descriptor)
		responseBytes, _ := json.Marshal(response)
		if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
			log.Errorf("Failed to write error response: %v", err)
		}
		return
	}

	log.Debugf("File descriptor sent successfully for %s (resolved from %s)", resolvedPath, req.Path)
}
