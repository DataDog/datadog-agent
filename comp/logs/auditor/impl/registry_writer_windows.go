// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package auditorimpl

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

func replaceRegistryFile(sourcePath, targetPath string) error {
	sourcePathPtr, err := windowsPathPtr(sourcePath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	targetPathPtr, err := windowsPathPtr(targetPath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}

	// Wait for the replacement itself to reach disk after the temporary file's
	// contents have been synchronized.
	err = windows.MoveFileEx(
		sourcePathPtr,
		targetPathPtr,
		windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH,
	)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	return nil
}

// windowsPathPtr preserves the long-path support normally provided by
// os.Rename before passing a path directly to a Windows API.
func windowsPathPtr(path string) (*uint16, error) {
	if isExtendedWindowsPath(path) || isWindowsDevicePath(path) {
		return windows.UTF16PtrFromString(path)
	}

	fullPath, err := windows.FullPath(path)
	if err != nil {
		return nil, err
	}
	if len(fullPath) < 248 {
		return windows.UTF16PtrFromString(path)
	}

	if strings.HasPrefix(fullPath, `\\`) {
		fullPath = `\\?\UNC\` + strings.TrimPrefix(fullPath, `\\`)
	} else {
		fullPath = `\\?\` + fullPath
	}
	return windows.UTF16PtrFromString(fullPath)
}

func isExtendedWindowsPath(path string) bool {
	return strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\??\`)
}

func isWindowsDevicePath(path string) bool {
	return strings.HasPrefix(path, `\\.\`)
}
