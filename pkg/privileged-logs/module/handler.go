// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
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

// openFileHandler opens the requested log file and passes its file descriptor
// to the client. Failures are plain HTTP error responses.
func (f *privilegedLogsModule) openFileHandler(w http.ResponseWriter, r *http.Request) {
	var req common.OpenFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendError(w, http.StatusBadRequest, fmt.Sprintf("Failed to parse request: %v", err))
		return
	}

	f.logFileAccess(req.Path)

	var file *os.File
	var err error
	if req.NoFollow {
		file, err = validateAndOpenNoFollow(req.Path)
	} else {
		file, err = validateAndOpen(req.Path)
	}
	if err != nil {
		sendError(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer file.Close()

	if err := sendFile(w, file); err != nil {
		log.Errorf("Failed to send file descriptor for %s: %v", req.Path, err)
	}
}

// sendFile switches the connection to common.UpgradeProtocol, sends the file
// descriptor as SCM_RIGHTS in a message of its own, and closes the connection.
func sendFile(w http.ResponseWriter, file *os.File) error {
	conn, _, err := http.NewResponseController(w).Hijack()
	if err != nil {
		return err
	}
	defer conn.Close()
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("not a Unix connection")
	}

	header := "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: " + common.UpgradeProtocol + "\r\n\r\n"
	if _, err := unixConn.Write([]byte(header)); err != nil {
		return err
	}
	// SCM_RIGHTS needs at least one byte of data to travel with.
	_, _, err = unixConn.WriteMsgUnix([]byte{0}, syscall.UnixRights(int(file.Fd())), nil)
	return err
}
