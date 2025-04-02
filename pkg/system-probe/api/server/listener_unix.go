// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build unix

package server

import (
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NewListener creates a Unix Domain Socket Listener
func NewListener(socketAddr string) (net.Listener, error) {
	if len(socketAddr) == 0 {
		return nil, errors.New("uds: empty socket path provided")
	}

	// Check to see if there's a pre-existing system probe socket.
	fileInfo, err := os.Stat(socketAddr)
	if err == nil { // No error means the socket file already exists
		// If it's not a UNIX socket, then this is a problem.
		if fileInfo.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("uds: reuse %s socket path: path already exists and it is not a UNIX socket", socketAddr)
		}
		// Attempt to remove the pre-existing socket
		if err = os.Remove(socketAddr); err != nil {
			return nil, fmt.Errorf("uds: remove stale UNIX socket: %v", err)
		}
	}

	conn, err := net.Listen("unix", socketAddr)
	if err != nil {
		return nil, fmt.Errorf("listen: %s", err)
	}

	if err := os.Chmod(socketAddr, 0720); err != nil {
		return nil, fmt.Errorf("socket chmod write-only: %s", err)
	}

	perms, err := filesystem.NewPermission()
	if err != nil {
		return nil, err
	}

	if err := perms.RestrictAccessToUser(socketAddr); err != nil {
		return nil, err
	}

	log.Debugf("uds: %s successfully initialized", conn.Addr())
	return conn, nil
}
