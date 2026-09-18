// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package demultiplexerendpointimpl

import (
	"errors"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

var replaceFileW = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReplaceFileW")

func replaceFile(sourcePath, destinationPath string) error {
	if _, err := os.Stat(destinationPath); errors.Is(err, os.ErrNotExist) {
		return os.Rename(sourcePath, destinationPath)
	} else if err != nil {
		return err
	}

	destination, err := windows.UTF16PtrFromString(destinationPath)
	if err != nil {
		return err
	}
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}

	success, _, callErr := replaceFileW.Call(
		uintptr(unsafe.Pointer(destination)),
		uintptr(unsafe.Pointer(source)),
		0,
		0,
		0,
		0,
	)
	if success == 0 {
		return os.NewSyscallError("ReplaceFileW", callErr)
	}
	return nil
}
