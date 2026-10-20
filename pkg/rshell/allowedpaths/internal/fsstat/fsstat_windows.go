// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package fsstat

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	fileFsVolumeInformation    = 1
	fileFsAttributeInformation = 5
	fileFsFullSizeInformation  = 7
	volumeInfoBufferSize       = 4096
)

var ntQueryVolumeInformationFile = windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQueryVolumeInformationFile")

// fileFsFullSizeInfo mirrors FILE_FS_FULL_SIZE_INFORMATION. Windows reports
// filesystem capacity in allocation units rather than POSIX blocks.
type fileFsFullSizeInfo struct {
	TotalAllocationUnits           int64
	CallerAvailableAllocationUnits int64
	ActualAvailableAllocationUnits int64
	SectorsPerAllocationUnit       uint32
	BytesPerSector                 uint32
}

func read(root *os.Root, relPath string, requireDirectory bool) (Info, error) {
	if !filepath.IsLocal(relPath) {
		return Info{}, &os.PathError{Op: "statfs", Path: relPath, Err: os.ErrInvalid}
	}

	rootFile, err := root.Open(".")
	if err != nil {
		return Info{}, err
	}
	defer rootFile.Close()

	handle := windows.Handle(rootFile.Fd())
	if cleanPath := filepath.Clean(relPath); cleanPath != "." {
		handle, err = openMetadataAt(handle, cleanPath)
		if err != nil {
			if err == windows.STATUS_REPARSE_POINT_ENCOUNTERED {
				return Info{}, ErrPathChanged
			}
			return Info{}, &os.PathError{Op: "statfs", Path: relPath, Err: ntStatusErr(err)}
		}
		defer windows.CloseHandle(handle)
	}

	if requireDirectory {
		var opened windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &opened); err != nil {
			return Info{}, &os.PathError{Op: "statfs", Path: relPath, Err: err}
		}
		if opened.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
			return Info{}, ErrNotDirectory
		}
	}

	info, err := readHandle(handle)
	if err != nil {
		return Info{}, &os.PathError{Op: "statfs", Path: relPath, Err: err}
	}
	return info, nil
}

// openMetadataAt opens path relative to rootHandle and rejects reparse points
// in every component. The caller has already resolved legitimate reparse
// points through AllowedPaths; encountering one here means that the path
// changed and must be resolved again.
func openMetadataAt(rootHandle windows.Handle, path string) (windows.Handle, error) {
	name, err := windows.NewNTUnicodeString(path)
	if err != nil {
		return windows.InvalidHandle, err
	}
	attrs := &windows.OBJECT_ATTRIBUTES{
		Length:        uint32(unsafe.Sizeof(windows.OBJECT_ATTRIBUTES{})),
		RootDirectory: rootHandle,
		ObjectName:    name,
		Attributes:    windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE,
	}

	var handle windows.Handle
	err = windows.NtCreateFile(
		&handle,
		windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		attrs,
		&windows.IO_STATUS_BLOCK{},
		nil,
		0,
		windows.FILE_SHARE_DELETE|windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		windows.FILE_OPEN,
		windows.FILE_OPEN_FOR_BACKUP_INTENT|windows.FILE_SYNCHRONOUS_IO_NONALERT,
		0,
		0,
	)
	return handle, err
}

func readHandle(handle windows.Handle) (Info, error) {
	fullSize, err := queryFullSize(handle)
	if err != nil {
		return Info{}, err
	}
	if fullSize.TotalAllocationUnits < 0 ||
		fullSize.CallerAvailableAllocationUnits < 0 ||
		fullSize.ActualAvailableAllocationUnits < 0 ||
		fullSize.SectorsPerAllocationUnit == 0 ||
		fullSize.BytesPerSector == 0 {
		return Info{}, fmt.Errorf("invalid filesystem size information")
	}

	id, idAvailable := queryVolumeID(handle)
	nameMax, nameMaxAvailable, typeName := queryFileSystemAttributes(handle)
	allocationUnitSize := uint64(fullSize.SectorsPerAllocationUnit) * uint64(fullSize.BytesPerSector)
	return Info{
		ID:                   id,
		IDAvailable:          idAvailable,
		NameMax:              nameMax,
		NameMaxAvailable:     nameMaxAvailable,
		TypeName:             typeName,
		IOBlockSize:          allocationUnitSize,
		FundamentalBlockSize: allocationUnitSize,
		Blocks:               uint64(fullSize.TotalAllocationUnits),
		BlocksFree:           uint64(fullSize.ActualAvailableAllocationUnits),
		BlocksAvailable:      uint64(fullSize.CallerAvailableAllocationUnits),
		FilesAvailable:       false,
	}, nil
}

func queryVolumeID(handle windows.Handle) (uint64, bool) {
	buffer := make([]byte, volumeInfoBufferSize)
	bytesReturned, err := queryVolumeInformation(
		handle,
		unsafe.Pointer(&buffer[0]),
		uintptr(len(buffer)),
		fileFsVolumeInformation,
	)
	if err == nil && bytesReturned >= 12 && bytesReturned <= uintptr(len(buffer)) {
		return uint64(binary.LittleEndian.Uint32(buffer[8:12])), true
	}

	// Retain a Win32 fallback for filesystems that do not implement the NT
	// volume-information class but do expose ordinary by-handle metadata.
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err == nil {
		return uint64(info.VolumeSerialNumber), true
	}
	return 0, false
}

func queryFileSystemAttributes(handle windows.Handle) (nameMax uint64, nameMaxAvailable bool, typeName string) {
	buffer := make([]byte, volumeInfoBufferSize)
	bytesReturned, err := queryVolumeInformation(
		handle,
		unsafe.Pointer(&buffer[0]),
		uintptr(len(buffer)),
		fileFsAttributeInformation,
	)
	const headerSize = 12
	if err != nil || bytesReturned < headerSize || bytesReturned > uintptr(len(buffer)) {
		return 0, false, ""
	}

	nameBytes := binary.LittleEndian.Uint32(buffer[8:12])
	if nameBytes%2 != 0 || uintptr(nameBytes) > bytesReturned-headerSize {
		return 0, false, ""
	}

	maximumComponentLength := int32(binary.LittleEndian.Uint32(buffer[4:8]))
	if maximumComponentLength > 0 {
		nameMax = uint64(maximumComponentLength)
		nameMaxAvailable = true
	}

	name := make([]uint16, int(nameBytes/2))
	for i := range name {
		offset := headerSize + i*2
		name[i] = binary.LittleEndian.Uint16(buffer[offset : offset+2])
	}
	return nameMax, nameMaxAvailable, windows.UTF16ToString(name)
}

func queryFullSize(handle windows.Handle) (fileFsFullSizeInfo, error) {
	var fullSize fileFsFullSizeInfo
	bytesReturned, err := queryVolumeInformation(
		handle,
		unsafe.Pointer(&fullSize),
		unsafe.Sizeof(fullSize),
		fileFsFullSizeInformation,
	)
	if err != nil {
		return fileFsFullSizeInfo{}, err
	}
	if bytesReturned < unsafe.Sizeof(fullSize) {
		return fileFsFullSizeInfo{}, fmt.Errorf("short filesystem size information")
	}
	return fullSize, nil
}

func queryVolumeInformation(
	handle windows.Handle,
	buffer unsafe.Pointer,
	bufferSize uintptr,
	informationClass uintptr,
) (uintptr, error) {
	var ioStatus windows.IO_STATUS_BLOCK
	status, _, _ := ntQueryVolumeInformationFile.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&ioStatus)),
		uintptr(buffer),
		bufferSize,
		informationClass,
	)
	if ntStatus := windows.NTStatus(uint32(status)); ntStatus != windows.STATUS_SUCCESS {
		return 0, ntStatus.Errno()
	}
	return ioStatus.Information, nil
}

func ntStatusErr(err error) error {
	if status, ok := err.(windows.NTStatus); ok {
		return status.Errno()
	}
	return err
}
