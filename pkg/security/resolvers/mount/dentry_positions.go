// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package mount holds mount related files
package mount

import (
	skernel "github.com/DataDog/datadog-agent/pkg/security/ebpf/kernel"
)

// dentryPosition returns the dentry position for a VFS operation based on kernel version.
// Kernel 5.12+ shifted dentry positions by 1 for several VFS operations.
func dentryPosition(kernelVersion *skernel.Version, base uint64) uint64 {
	if kernelVersion.Code != 0 && kernelVersion.Code >= skernel.Kernel5_12 {
		return base + 1
	}
	return base
}

// GetVFSLinkDentryPosition gets VFS link dentry position
func GetVFSLinkDentryPosition(kernelVersion *skernel.Version) uint64 {
	return dentryPosition(kernelVersion, 2)
}

// GetVFSMKDirDentryPosition gets VFS MKDir dentry position
func GetVFSMKDirDentryPosition(kernelVersion *skernel.Version) uint64 {
	return dentryPosition(kernelVersion, 2)
}

// GetVFSSetxattrDentryPosition gets VFS set xattr dentry position
func GetVFSSetxattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	return dentryPosition(kernelVersion, 1)
}

// GetVFSRemovexattrDentryPosition gets VFS remove xattr dentry position
func GetVFSRemovexattrDentryPosition(kernelVersion *skernel.Version) uint64 {
	return dentryPosition(kernelVersion, 1)
}
