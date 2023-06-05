// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package replay

import (
	"errors"
	"os"
	"reflect"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type memoryMap []byte

// keep track of handles
var (
	handleLock sync.Mutex
	handleMap  = map[uintptr]windows.Handle{}
)

func (m *memoryMap) header() *reflect.SliceHeader {
	return (*reflect.SliceHeader)(unsafe.Pointer(m))
}

// getFileContent returns a slice of bytes with the contents of the file specified in the path.
// The mmap flag will try to Map the file so as to achieve reasonable performance with very large
// files while not loading the entire thing into memory.
func getFileContent(path string, mmap bool) ([]byte, error) {

	if !mmap {
		return os.ReadFile(path)
	}

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
	h, errno := windows.CreateFileMapping(windows.Handle(int(f.Fd())), nil,
		uint32(windows.PAGE_READONLY), 0, uint32(size), nil)
	if h == 0 {
		return nil, os.NewSyscallError("CreateFileMapping", errno)
	}
	// Create the memory map.
	addr, errno := windows.MapViewOfFile(h, uint32(windows.FILE_MAP_READ), 0, 0, uintptr(size))
	if addr == 0 {
		return nil, os.NewSyscallError("MapViewOfFile", errno)
	}

	handleLock.Lock()
	handleMap[addr] = h
	handleLock.Unlock()

	// Convert to a byte array.
	m := memoryMap{}
	dh := m.header()
	dh.Data = addr
	dh.Len = size
	dh.Cap = dh.Len

	return m, nil

}

func flush(addr, len uintptr) error {
	errno := windows.FlushViewOfFile(addr, len)
	return os.NewSyscallError("FlushViewOfFile", errno)
}

func unmapFile(b []byte) error {
	m := memoryMap(b)
	dh := m.header()

	addr := dh.Data
	size := uintptr(dh.Len)

	err := flush(addr, size)
	if err != nil {
		// continue and unmap the file even if there's an error
		log.Warnf("There was a non-fatal issue flushing the file map: %v", err)
	}

	err = windows.UnmapViewOfFile(addr)
	if err != nil {
		return err
	}

	handleLock.Lock()
	defer handleLock.Unlock()
	handle, ok := handleMap[addr]
	if !ok {
		// should be impossible; we would've errored above
		return errors.New("unknown base address")
	}
	delete(handleMap, addr)

	e := windows.CloseHandle(windows.Handle(handle))
	return os.NewSyscallError("CloseHandle", e)
}
