// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package cgroups

// From https://github.com/torvalds/linux/blob/5859a2b1991101d6b978f3feb5325dad39421f29/include/linux/proc_ns.h#L41-L49
// Currently, host namespace inode number are hardcoded, which can be used to detect
// if we're running in host namespace or not (does not work when running in DinD)
const (
	hostCgroupNamespaceInode = 0xEFFFFFFB
)

// IsProcessHostCgroupNamespace compares namespaceID with known, harcoded host PID Namespace inode
// Keeps same signature as `IsProcessHostNetwork` as we may need to change implementation depending on Kernel evolution
func IsProcessHostCgroupNamespace(procPath string, namespaceID uint64) *bool {
	b := namespaceID == hostCgroupNamespaceInode
	return &b
}
