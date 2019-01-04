// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package file

import (
	"os"
	"syscall"
)

// openFile reimplements the os.Open function for Windows because the default
// implementation opens files without the FILE_SHARE_DELETE flag.
// cf: https://github.com/golang/go/blob/release-branch.go1.11/src/syscall/syscall_windows.go#L271
// This prevents users from moving/removing files when the tailer is reading the file.
func openFile(path string) (*os.File, error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	access := uint32(syscall.GENERIC_READ)
	// add FILE_SHARE_DELETE that is missing from os.Open implementation
	sharemode := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE)
	createmode := uint32(syscall.OPEN_EXISTING)
	var sa *syscall.SecurityAttributes

	r, err := syscall.CreateFile(pathp, access, sharemode, sa, createmode, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(r), path), nil
}
