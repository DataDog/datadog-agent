// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package helpers

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// openArchive opens the archive for overlapped I/O so a read deadline can end a blocked read, and shares delete
// access so a failed upload's archive can be removed while it is still open.
func openArchive(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(extendedLengthPath(path))
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	shareMode := uint32(windows.FILE_SHARE_READ | windows.FILE_SHARE_WRITE | windows.FILE_SHARE_DELETE)
	h, err := windows.CreateFile(p, windows.GENERIC_READ, shareMode, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}

// extendedLengthPath adds the \\?\ prefix to a path too long for CreateFile, as os.Open does.
func extendedLengthPath(path string) string {
	if hasDevicePrefix(path) {
		return path
	}
	full, err := filepath.Abs(path)
	// CreateFile accepts full paths shorter than MAX_PATH minus room for an 8.3 name (260 - 12).
	if err != nil || len(full) < 248 {
		return path
	}
	if strings.HasPrefix(full, `\\`) {
		return `\\?\UNC\` + full[2:]
	}
	return `\\?\` + full
}

// hasDevicePrefix reports whether path starts with \??\, or with \\?\ or \\.\ in either separator style; none takes the prefix.
func hasDevicePrefix(path string) bool {
	if strings.HasPrefix(path, `\??\`) {
		return true
	}
	return len(path) >= 4 && os.IsPathSeparator(path[0]) && os.IsPathSeparator(path[1]) &&
		(path[2] == '?' || path[2] == '.') && os.IsPathSeparator(path[3])
}
