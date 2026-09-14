// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package auditorimpl

import (
	"os"

	"golang.org/x/sys/windows"
)

func replaceRegistryFile(sourcePath, targetPath string) error {
	sourcePathPtr, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	targetPathPtr, err := windows.UTF16PtrFromString(targetPath)
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
