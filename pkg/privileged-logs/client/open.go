// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

// Package client provides functionality to open files through the privileged logs module.
package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"syscall"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/privileged-logs/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// OpenPrivileged opens a file through system-probe.
func OpenPrivileged(socketPath string, filePath string) (*os.File, error) {
	return openPrivileged(socketPath, filePath, false)
}

// OpenPrivilegedNoFollow opens a file in system-probe without following symbolic
// links in any path component.
func OpenPrivilegedNoFollow(socketPath string, filePath string) (*os.File, error) {
	return openPrivileged(socketPath, filePath, true)
}

func openPrivileged(socketPath string, filePath string, noFollow bool) (*os.File, error) {
	// A connection of its own, since the server takes it over to pass the file
	// descriptor.
	conn, err := net.DialUnix("unix", nil, &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to system-probe: %v", err)
	}
	defer conn.Close()

	body, err := json.Marshal(common.OpenFileRequest{Path: filePath, NoFollow: noFollow})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, "http://sysprobe/privileged_logs/open", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	// On success the server switches protocols and sends the file descriptor;
	// otherwise "close" makes it close the connection after its response.
	// Either way, the reply ends at EOF.
	req.Header.Set("Connection", "Upgrade, close")
	req.Header.Set("Upgrade", common.UpgradeProtocol)
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("failed to write request: %v", err)
	}

	reply, fds, err := readWithRights(conn)
	defer func() {
		for _, fd := range fds {
			syscall.Close(fd)
		}
	}()
	if err != nil {
		return nil, err
	}

	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(reply)), req)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSwitchingProtocols {
		message, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("module error: %s", bytes.TrimSpace(message))
	}
	if len(fds) != 1 {
		return nil, fmt.Errorf("expected 1 file descriptor, got %d", len(fds))
	}

	file := os.NewFile(uintptr(fds[0]), filePath)
	fds = nil
	return file, nil
}

// readWithRights reads conn until EOF, collecting the file descriptors passed
// as SCM_RIGHTS. It only uses recvmsg, since the kernel discards the
// descriptors attached to bytes consumed by a plain read.
func readWithRights(conn *net.UnixConn) ([]byte, []int, error) {
	var data []byte
	var fds []int
	buf := make([]byte, 1024)
	oob := make([]byte, syscall.CmsgSpace(4))
	for {
		n, oobn, _, _, err := conn.ReadMsgUnix(buf, oob)
		if errors.Is(err, io.EOF) {
			return data, fds, nil
		}
		if err != nil {
			return data, fds, fmt.Errorf("ReadMsgUnix failed: %v", err)
		}
		data = append(data, buf[:n]...)
		msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
		if err != nil {
			return data, fds, fmt.Errorf("ParseSocketControlMessage failed: %v", err)
		}
		for _, msg := range msgs {
			if msg.Header.Level != syscall.SOL_SOCKET || msg.Header.Type != syscall.SCM_RIGHTS {
				continue
			}
			rights, err := syscall.ParseUnixRights(&msg)
			if err != nil {
				return data, fds, fmt.Errorf("ParseUnixRights failed: %v", err)
			}
			fds = append(fds, rights...)
		}
	}
}

func maybeOpenPrivileged(path string, originalError error, noFollow bool) (*os.File, error) {
	enabled := pkgconfigsetup.SystemProbe().GetBool("privileged_logs.enabled")
	if !enabled {
		return nil, originalError
	}

	log.Debugf("Permission denied, opening file with system-probe: %v", path)

	socketPath := pkgconfigsetup.SystemProbe().GetString("system_probe_config.sysprobe_socket")
	file, spErr := openPrivileged(socketPath, path, noFollow)
	log.Tracef("Opened file with system-probe: %v, err: %v", path, spErr)
	if spErr != nil {
		return nil, fmt.Errorf("failed to open file with system-probe: %w, original error: %w", spErr, originalError)
	}

	return file, nil
}

// Open opens a file directly or through system-probe when permission is denied.
func Open(path string) (*os.File, error) {
	return open(path, false)
}

// OpenNoFollow is like Open but rejects symbolic links in every path component.
func OpenNoFollow(path string) (*os.File, error) {
	return open(path, true)
}

func open(path string, noFollow bool) (*os.File, error) {
	var file *os.File
	var err error

	if noFollow {
		file, err = common.OpenPathWithoutSymlinks(path)
	} else {
		file, err = os.Open(path)
	}
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return file, err
	}

	return maybeOpenPrivileged(path, err, noFollow)
}

// Stat attempts to stat a file, and if it fails due to permissions, it opens
// the file using system-probe if the privileged logs module is available and
// stats the opened file.
func Stat(path string) (os.FileInfo, error) {
	info, err := os.Stat(path)
	if err == nil || !errors.Is(err, os.ErrPermission) {
		return info, err
	}

	file, err := maybeOpenPrivileged(path, err, false)
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
