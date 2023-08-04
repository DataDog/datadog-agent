// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// From https://github.com/torvalds/linux/blob/5859a2b1991101d6b978f3feb5325dad39421f29/include/linux/proc_ns.h#L41-L49
// Currently, host namespace inode number are hardcoded, which can be used to detect
// if we're running in host namespace or not (does not work when running in DinD)
const (
	hostUTSNamespecInode = 0xEFFFFFFE
)

var (
	netNSPid1     uint64
	syncNetNSPid1 sync.Once
)

// GetProcessNamespaceInode performs a stat() call on /proc/<pid>/ns/<namespace>
func GetProcessNamespaceInode(procPath string, pid string, namespace string) (uint64, error) {
	nsPath := filepath.Join(procPath, pid, "ns", namespace)
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
		netNSPid1, _ = GetProcessNamespaceInode(procPath, "1", "net")
	})

	if netNSPid1 == 0 {
		return nil
	}

	res := netNSPid1 == namespaceID
	return &res
}

// IsProcessHostUTSNamespace compares namespaceID with known, harcoded host PID Namespace inode
// Keeps same signature as `IsProcessHostNetwork` as we may need to change implementation depending on Kernel evolution
func IsProcessHostUTSNamespace(procPath string, namespaceID uint64) *bool {
	return pointer.Ptr(namespaceID == hostUTSNamespecInode)
}
