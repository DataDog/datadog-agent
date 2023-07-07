// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package net

import (
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"syscall"

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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

// IsUnixNetConnValid return true if the connection is an unix socket
// and client binary sha256 match with sig (use client pid as source of truth)
func IsUnixNetConnValid(unixConn *net.UnixConn, sig string) (bool, error) {
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

	f, err := os.Open(util.HostProc(strconv.FormatUint(uint64(ucred.Pid), 10), "exe"))
	if err != nil {
		return false, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, err
	}
	if fmt.Sprintf("%x", h.Sum(nil)) == sig {
		return true, nil
	}

	exepath, _ := os.Readlink(util.HostProc(strconv.FormatUint(uint64(ucred.Pid), 10), "exe"))
	log.Debugf("rejected %s expected %s pid %s path %s", fmt.Sprintf("%x", h.Sum(nil)), sig, ucred.Pid, exepath)
	return false, nil
}
