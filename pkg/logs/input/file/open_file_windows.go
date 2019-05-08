// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

// +build windows

package file

import (
	"os"

	"golang.org/x/sys/windows"
)

// openFile reimplements the os.Open function for Windows because the default
// implementation opens files without the FILE_SHARE_DELETE flag.
// cf: https://github.com/golang/go/blob/release-branch.go1.11/src/syscall/syscall_windows.go#L271
// This prevents users from moving/removing files when the tailer is reading the file.
func openFile(path string) (*os.File, error) {
	pathp, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	access := uint32(windows.GENERIC_READ)
	// add FILE_SHARE_DELETE that is missing from os.Open implementation
	sharemode := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	createmode := uint32(windows.OPEN_EXISTING)
	var sa *windows.SecurityAttributes

	r, err := windows.CreateFile(pathp, access, sharemode, sa, createmode, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}

	return os.NewFile(uintptr(r), path), nil
}
