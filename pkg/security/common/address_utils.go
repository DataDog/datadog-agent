// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common holds common related files
package common

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// GetFamilyAddress returns the address famility to use for system-probe <-> security-agent communication
func GetFamilyAddress(path string) string {
	if strings.HasPrefix(path, "/") {
		return "unix"
	}
	return "tcp"
}

// GetCmdSocketPath returns the path to the cmd socket for system-probe <-> security-agent communication
// it will use the cmd_socket config if set, otherwise it will use the socket config as a base
func GetCmdSocketPath(socketPath string, cmdSocketPath string) (string, error) {
	if cmdSocketPath == "" {
		if socketPath == "" {
			return "", errors.New("runtime_security_config.cmd_socket or runtime_security_config.socket must be set")
		}

		family := GetFamilyAddress(socketPath)
		if family == "unix" {
			if runtime.GOOS == "windows" {
				return "", errors.New("unix sockets are not supported on Windows")
			}

			socketDir, socketName := filepath.Split(socketPath)

			cmdSocketPath = fmt.Sprintf("%scmd-%s", socketDir, socketName)
		} else {
			addrPort := strings.Split(socketPath, ":")
			if len(addrPort) != 2 {
				return "", fmt.Errorf("invalid socket path: %s", socketPath)
			}

			port, err := strconv.Atoi(addrPort[1])
			if err != nil {
				return "", fmt.Errorf("invalid socket port: %s", addrPort[1])
			}

			cmdSocketPath = addrPort[0] + ":" + strconv.Itoa(port+1)
		}
	}

	return cmdSocketPath, nil
}
