// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package demultiplexerendpointimpl

import (
	"os"

	"golang.org/x/sys/windows"
)

func replaceFile(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}

	if err := windows.MoveFileEx(source, destination, windows.MOVEFILE_REPLACE_EXISTING); err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: destinationPath, Err: err}
	}
	return nil
}
