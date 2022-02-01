// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package system

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
)

var (
	netNSPid1     uint64
	syncNetNSPid1 sync.Once
)

// GetProcessNamespaceInode performs a stat() call on /proc/<pid>/ns/<namespace>
func GetProcessNamespaceInode(procPath string, pid int, namespace string) (uint64, error) {
	nsPath := filepath.Join(procPath, strconv.Itoa(pid), "ns", namespace)
	fi, err := os.Stat(nsPath)
	if err != nil {
		return 0, err
	}

	// We are on linux, casting in safe
	return fi.Sys().(*syscall.Stat_t).Ino, nil
}

// IsProcessHostNetwork compares namespaceID (inode behind /proc/<pid>/ns/net returned by GetProcessNamespaceInode)
// to  PID 1 namespace id, which we assume runs in host network namespace
func IsProcessHostNetwork(procPath string, namespaceID uint64) *bool {
	syncNetNSPid1.Do(func() {
		netNSPid1, _ = GetProcessNamespaceInode(procPath, 1, "net")
	})

	if netNSPid1 == 0 {
		return nil
	}

	res := netNSPid1 == namespaceID
	return &res
}
