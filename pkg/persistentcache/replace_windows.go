// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package persistentcache

import (
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

// Match the logs registry writer's write-through replacement: synchronizing
// the temporary file alone does not make the directory update durable across
// an abrupt Windows shutdown.
func replaceFile(sourcePath, targetPath string) error {
	source, err := renamePathPtr(sourcePath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	target, err := renamePathPtr(targetPath)
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	if err := windows.MoveFileEx(source, target, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: targetPath, Err: err}
	}
	return nil
}

// Preserve the long-path handling os.Rename normally provides.
func renamePathPtr(path string) (*uint16, error) {
	if strings.HasPrefix(path, `\\?\`) || strings.HasPrefix(path, `\??\`) || strings.HasPrefix(path, `\\.\`) {
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
