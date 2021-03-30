// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build windows

package replay

import (
	"os"
	"reflect"
	"syscall"
	"unsafe"
)

const maxMapSize = 0xFFFFFFFFFFFF // 256TB

type memoryMap []byte

func (m *memoryMap) header() *reflect.SliceHeader {
	return (*reflect.SliceHeader)(unsafe.Pointer(m))
}

func getFileMap(path string) ([]byte, error) {

	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := int(stat.Size())

	// Open a file mapping handle.
	sizelo := uint32(size >> 32)
	sizehi := uint32(size) & 0xffffffff

	h, errno := syscall.CreateFileMapping(syscall.Handle(int(f.Fd())), nil, syscall.PAGE_READONLY, sizelo, sizehi, nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}
	// Create the memory map.
	addr, errno := syscall.MapViewOfFile(h, syscall.FILE_MAP_READ, 0, 0, uintptr(size))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}
	// Close mapping handle.
	if err := syscall.CloseHandle(syscall.Handle(h)); err != nil {
		return nil, os.NewSyscallError("CloseHandle", err)
	}

	// Convert to a byte array.
	m := memoryMap{}
	dh := m.header()
	dh.Data = addr
	dh.Len = size
	dh.Cap = dh.Len

	return m, nil

}
