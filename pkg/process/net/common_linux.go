// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

import (
	"fmt"
	"net"
	"os"
	"syscall"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
)

const (
	connectionsURL = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/connections"
	procStatsURL   = "http://unix/" + string(sysconfig.ProcessModule) + "/stats"
	registerURL    = "http://unix/" + string(sysconfig.NetworkTracerModule) + "/register"
	statsURL       = "http://unix/debug/stats"
	netType        = "unix"
)

// CheckPath is used in conjunction with calling the stats endpoint, since we are calling this
// From the main agent and want to ensure the socket exists
func CheckPath(path string) error {
	if path == "" {
		return fmt.Errorf("socket path is empty")
	}

	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("socket path does not exist: %v", err)
	}
	return nil
}

// IsUnixNetConnValid return true id the connection is an unix socket
// and client of the connection is root:root or allowedUsrID:allowedGrpID
func IsUnixNetConnValid(unixConn *net.UnixConn, allowedUsrID int, allowedGrpID int) (bool, error) {
	sysConn, err := unixConn.SyscallConn()
	if err != nil {
		return false, err
	}
	var ucred *syscall.Ucred
	var ucredErr error
	err = sysConn.Control(func(fd uintptr) {
		ucred, ucredErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	})
	if err != nil {
		return false, err
	}
	if ucredErr != nil {
		return false, ucredErr
	}
	if (ucred.Uid == 0 && ucred.Gid == 0) ||
		(ucred.Uid == uint32(allowedUsrID) && ucred.Gid == uint32(allowedGrpID)) {
		return true, nil
	}
	return false, nil
}
