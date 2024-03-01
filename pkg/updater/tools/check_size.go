// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tools contains tooling required by the updater.
package tools

import "github.com/DataDog/datadog-agent/pkg/util/filesystem"

type disk interface {
	GetUsage(path string) (*filesystem.DiskUsage, error)
}

// CheckAvailableDiskSpace checks if the given path has enough free space to store the required bytes
// This will check the underlying partition of the given path. Note that the path must be an existing dir.
//
// In the underlying filesystem package, disk.Free is used to check the available disk space
// On Unix, it is computed using `statfs` and is the number of free blocks available to an unprivileged used * block size
// See https://man7.org/linux/man-pages/man2/statfs.2.html for more details
// On Windows, it is computed using `GetDiskFreeSpaceExW` and is the number of bytes available
// See https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getdiskfreespaceexw for more details
func CheckAvailableDiskSpace(disk disk, path string, requiredBytes uint64) (bool, error) {
	s, err := disk.GetUsage(path)
	if err != nil {
		return false, err
	}
	return s.Available >= requiredBytes, nil
}
