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

type fileRenameInfo struct {
	flags          uint32
	rootDirectory  windows.Handle
	fileNameLength uint32
	fileName       [1]uint16
}

func replaceFile(sourcePath, destinationPath string) error {
	source, err := windows.UTF16PtrFromString(sourcePath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16FromString(destinationPath)
	if err != nil {
		return err
	}

	err = replaceFileWithPOSIXSemantics(source, destination)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) || errors.Is(err, windows.ERROR_NOT_SUPPORTED) {
		err = windows.MoveFileEx(source, &destination[0], windows.MOVEFILE_REPLACE_EXISTING)
	}
	if err != nil {
		return &os.LinkError{Op: "rename", Old: sourcePath, New: destinationPath, Err: err}
	}
	return nil
}

func replaceFileWithPOSIXSemantics(source *uint16, destination []uint16) error {
	sourceHandle, err := windows.CreateFile(
		source,
		windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(sourceHandle)

	fileNameLength := (len(destination) - 1) * 2
	var renameInfo fileRenameInfo
	bufferSize := int(unsafe.Offsetof(renameInfo.fileName)) + fileNameLength
	buffer := make([]byte, bufferSize)
	renameInfoPtr := (*fileRenameInfo)(unsafe.Pointer(&buffer[0]))
	renameInfoPtr.flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	renameInfoPtr.fileNameLength = uint32(fileNameLength)
	copy(unsafe.Slice(&renameInfoPtr.fileName[0], len(destination)-1), destination)

	return windows.SetFileInformationByHandle(sourceHandle, windows.FileRenameInfoEx, &buffer[0], uint32(len(buffer)))
}
