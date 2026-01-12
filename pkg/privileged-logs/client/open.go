// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"errors"
	"os"

	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"syscall"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OpenPrivileged opens a file in system-probe and returns the file descriptor
// This function uses a custom HTTP client that can handle file descriptor transfer
func OpenPrivileged(socketPath string, filePath string) (*os.File, error) {
	// Create a new connection instead of reusing the shared connection from
	// pkg/system-probe/api/client/client.go, since the connection is hijacked
	// from the control of the HTTP server library on the server side.  It also
	// ensures that we don't affect other clients if something goes wrong with
	// our OOB handling leaving the connection unusable.
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system-probe: %v", err)
	}
	defer conn.Close()

	req := common.OpenFileRequest{
		Path: filePath,
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	httpReq, err := http.NewRequest("POST", "http://sysprobe/privileged_logs/open", bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request: %v", err)
	}

	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, errors.New("not a Unix connection")
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
		return nil, errors.New("no response received")
	}

	var response common.OpenFileResponse
	if err := json.Unmarshal(buf[:n], &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("module error: %s", response.Error)
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
					log.Tracef("Received file descriptor: %d", fd)
					return os.NewFile(uintptr(fd), filePath), nil
				}
			}
		}
	}

	return nil, errors.New("no file descriptor received")
}

func maybeOpenPrivileged(path string, originalError error) (*os.File, error) {
	enabled := pkgconfigsetup.SystemProbe().GetBool("privileged_logs.enabled")
	if !enabled {
		return nil, originalError
	}

	log.Debugf("Permission denied, opening file with system-probe: %v", path)

	socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
	file, spErr := OpenPrivileged(socketPath, path)
	log.Tracef("Opened file with system-probe: %v, err: %v", path, spErr)
	if spErr != nil {
		return nil, fmt.Errorf("failed to open file with system-probe: %w, original error: %w", spErr, originalError)
	}

	return file, nil
}

// Open attempts to open a file, and if it fails due to permissions, it opens
// the file using system-probe if the privileged logs module is available.
func Open(path string) (*os.File, error) {
	file, err := os.Open(path)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return file, err
	}

	file, err = maybeOpenPrivileged(path, err)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// Stat attempts to stat a file, and if it fails due to permissions, it opens
// the file using system-probe if the privileged logs module is available and
// stats the opened file.
func Stat(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return info, err
	}

	file, err := maybeOpenPrivileged(path, err)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	info, err = file.Stat()
	if err != nil {
		return nil, err
	}

	return info, nil
}
