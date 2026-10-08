// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// logFileAccess informs about uses of this endpoint.  To avoid frequent logging
// for the same files (log rotation detection in the core agent tries to open
// tailed files every 10 seconds), we only log the first access for each path.
func (f *privilegedLogsModule) logFileAccess(path string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.informedPaths != nil {
		if _, found := f.informedPaths.Get(path); found {
			return
		}

		f.informedPaths.Add(path, struct{}{})
	}

	log.Infof("Received request to open file: %s", path)
}

// sendError sends an error response to the client and logs the error
func sendError(w http.ResponseWriter, code int, message string) {
	log.Error(message)
	http.Error(w, message, code)
}

// openFileHandler handles requests to open a file and transfer its file descriptor
func (f *privilegedLogsModule) openFileHandler(w http.ResponseWriter, r *http.Request) {
	// We need to read the body fully before hijacking the connection
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Errorf("Failed to read body: %v", err)
		return
	}

	var req common.OpenFileRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	f.logFileAccess(req.Path)

	file, err := openLogFile(req.Path, req.NoFollow)
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer file.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
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
		return
	}

	// The status line and the file descriptor are sent in separate writes, as
	// system-probe-lite does, so the client must handle split delivery.
	if _, err := unixConn.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: dd-privileged-logs\r\n\r\n")); err != nil {
		log.Errorf("Failed to write response: %v", err)
		return
	}

	fd := int(file.Fd())
	log.Tracef("Sending file descriptor %d for file %s", fd, req.Path)

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

	log.Tracef("File descriptor sent successfully for %s", req.Path)
}
