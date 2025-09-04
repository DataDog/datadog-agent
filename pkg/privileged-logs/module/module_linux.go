// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package module implements the privileged logs module for the system-probe.
package module

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewPrivilegedLogsModule creates a new instance of the privileged logs module.
var NewPrivilegedLogsModule = func() module.Module {
	return &privilegedLogsModule{}
}

var _ module.Module = &privilegedLogsModule{}

type privilegedLogsModule struct{}

// GetStats returns stats for the module
func (f *privilegedLogsModule) GetStats() map[string]interface{} {
	return nil
}

// Register registers endpoints for the module to expose data
func (f *privilegedLogsModule) Register(httpMux *module.Router) error {
	httpMux.HandleFunc("/open", f.openFileHandler).Methods("POST")
	return nil
}

// Close cleans up the module
func (f *privilegedLogsModule) Close() {
	// No cleanup needed
}

// isLogFile checks if the given path ends with .log
func isLogFile(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".log")
}

// sendErrorResponse sends an error response to the client and logs the error
func (f *privilegedLogsModule) sendErrorResponse(unixConn *net.UnixConn, message string) {
	log.Errorf(message)
	response := common.OpenFileResponse{
		Success: false,
		Error:   message,
	}
	responseBytes, _ := json.Marshal(response)
	if _, _, err := unixConn.WriteMsgUnix(responseBytes, nil, nil); err != nil {
		log.Errorf("Failed to write error response: %v", err)
	}
}

func validateAndOpen(path string) (*os.File, error) {
	return validateAndOpenWithPrefix(path, "/var/log")
}

// isTextFile checks if the given file is a text file by reading the first 128 bytes
// and checking if they are valid UTF-8.  Note that empty files are considered
// text files.
func isTextFile(file *os.File) bool {
	buf := make([]byte, 128)
	// ReadAt ensures that the file offset is not modified.
	_, err := file.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		return false
	}
	return utf8.Valid(buf)
}

func validateAndOpenWithPrefix(path, allowedPrefix string) (*os.File, error) {
	if path == "" {
		return nil, fmt.Errorf("empty file path provided")
	}

	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("relative path not allowed: %s", path)
	}

	// Resolve symbolic links for the prefix and suffix checks. The OpenInRoot and
	// O_NOFOLLOW below protect against TOCTOU attacks.
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve path: %v", err)
	}

	var file *os.File
	if isLogFile(resolvedPath) {
		// Files ending with .log are allowed regardless of where they are
		// located in the file system, so we don't need to protect againt
		// symlink attacks for the components of the path.  For example, if the
		// path /var/log/foo/bar.log now points to /etc/bar.log (/var/log/foo ->
		// /etc), it's still a valid log file.
		//
		// We still do need to verify that the last component is still not a
		// symbolic link, O_NOFOLLOW ensures this.  For example, if
		// /var/log/foo/bar.log now points to /etc/shadow (bar.log ->
		// /etc/shadow), it should be prevented from being opened.
		file, err = os.OpenFile(resolvedPath, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	} else if strings.HasPrefix(resolvedPath, allowedPrefix) {
		// Files not ending with .log are only allowed if they are in
		// allowedPrefix.  OpenInRoot expects a path relative to the base
		// directory.
		relativePath := resolvedPath[len(allowedPrefix):]

		// OpenInRoot ensures that the path cannot escape the /var/log directory
		// (expanding symlinks, but protecting against symlink attacks).
		file, err = os.OpenInRoot(allowedPrefix, relativePath)
	} else {
		err = fmt.Errorf("non-log file not allowed")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", path, err)
	}

	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file %s: %v", path, err)
	}

	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("file %s is not a regular file", path)
	}

	if !isTextFile(file) {
		file.Close()
		return nil, errors.New("not a text file")
	}

	return file, nil
}

// openFileHandler handles requests to open a file and transfer its file descriptor
func (f *privilegedLogsModule) openFileHandler(w http.ResponseWriter, r *http.Request) {
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
		f.sendErrorResponse(unixConn, "Not a Unix connection")
		return
	}

	var req common.OpenFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		f.sendErrorResponse(unixConn, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	file, err := validateAndOpen(req.Path)
	if err != nil {
		f.sendErrorResponse(unixConn, err.Error())
		return
	}
	defer file.Close()

	fd := int(file.Fd())
	log.Debugf("Sending file descriptor %d for file %s", fd, req.Path)

	response := common.OpenFileResponse{
		Success: true,
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Errorf("Failed to marshal response: %v", err)
		return
	}

	rights := syscall.UnixRights(fd)
	_, _, err = unixConn.WriteMsgUnix(responseBytes, rights, nil)
	if err != nil {
		log.Errorf("WriteMsgUnix failed: %v", err)
		return
	}

	log.Debugf("File descriptor sent successfully for %s", req.Path)
}
