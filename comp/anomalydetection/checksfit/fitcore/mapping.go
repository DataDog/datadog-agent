// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`) at commit
// 4961722de9009afdbbb711fc0adf14a8f6ff9277 (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
//
// Local changes: the darwin build requires cgo, unsupported platforms get a
// stub so this tree still compiles, and test files carry an explicit platform
// gate. The transport logic, framing, and ring layout are unchanged.

//go:build linux || (darwin && cgo)

package fitcore

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

// mapping owns one mmap of the shared-memory object. The region slice keeps
// the mapping alive; it never contains Go pointers and the GC never moves it.
type mapping struct {
	fd       int
	region   []byte
	name     string // shm object name while this handle still owns the unlink
	capacity int
}

// word returns the 32-bit atomic alias at a fixed offset. mmap is
// page-aligned and both fixed offsets meet the four-byte atomic alignment.
func (m *mapping) word(offset int) *uint32 {
	return (*uint32)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(m.region)), offset))
}

// ring returns the raw ring bytes starting at ringOffset.
func (m *mapping) ring() []byte {
	return m.region[ringOffset : ringOffset+m.capacity]
}

// createMapping creates a fresh POSIX shared-memory object, maps it, and
// initializes the immutable metadata, both indexes, and the ring to zero
// before the object name is disclosed to the peer.
func createMapping(id uint64, capacity int, protocolVersion uint32) (*mapping, string, error) {
	if err := validateCapacity(capacity); err != nil {
		return nil, "", err
	}
	regionSize := ringOffset + capacity
	// Keep the name within macOS's short POSIX shared-memory name limit.
	name := fmt.Sprintf("/mc-%x-%016x", os.Getpid(), id)
	fd, err := shmOpen(name, syscall.O_CREAT|syscall.O_EXCL|syscall.O_RDWR, 0o600)
	if err != nil {
		return nil, "", err
	}
	if err := syscall.Ftruncate(fd, int64(regionSize)); err != nil {
		syscall.Close(fd)
		shmUnlink(name)
		return nil, "", err
	}
	shared, err := mapShared(fd, name, capacity)
	if err != nil {
		return nil, "", err
	}
	// The fresh object is already zero-filled; zero explicitly so the
	// metadata write starts from a known state, like the Rust template.
	for index := range shared.region {
		shared.region[index] = 0
	}
	writeMetadata(shared.region[:headerSize], id, capacity, protocolVersion)
	return shared, name, nil
}

// openMapping maps the offered object on the producer side and validates its
// backing size, owner, mode, header, and fresh zeroed queue region.
func openMapping(name string, id uint64, capacity int, protocolVersion uint32) (*mapping, error) {
	if err := validateCapacity(capacity); err != nil {
		return nil, err
	}
	regionSize := ringOffset + capacity
	if !strings.HasPrefix(name, "/mc-") || len(name) > 30 || strings.Contains(name[1:], "/") || strings.IndexByte(name, 0) >= 0 {
		return nil, invalid("invalid shared-memory name")
	}
	fd, err := shmOpen(name, syscall.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	var stat syscall.Stat_t
	if err := syscall.Fstat(fd, &stat); err != nil {
		syscall.Close(fd)
		return nil, err
	}
	expectedSize, err := shmExpectedBackingSize(regionSize)
	if err != nil {
		syscall.Close(fd)
		return nil, err
	}
	if stat.Size != expectedSize || stat.Uid != uint32(syscall.Geteuid()) || !shmModeAllowed(uint32(stat.Mode)) {
		syscall.Close(fd)
		return nil, fmt.Errorf(
			"shared-memory metadata mismatch: backing size=%d (expected %d), owner=%d (expected %d), mode=%o",
			stat.Size, expectedSize, stat.Uid, syscall.Geteuid(), stat.Mode&0o777)
	}
	shared, err := mapShared(fd, "", capacity)
	if err != nil {
		return nil, err
	}
	expected := make([]byte, headerSize)
	writeMetadata(expected, id, capacity, protocolVersion)
	if !bytes.Equal(shared.region[:headerSize], expected) || anyNonZero(shared.region[headerSize:]) {
		shared.close()
		return nil, invalid("shared-memory header or fresh queue region is invalid")
	}
	return shared, nil
}

// unlinkName removes the creator-owned shm object name after the producer
// mapped it; both peers keep their mapping for their local session.
func (m *mapping) unlinkName() error {
	if m.name == "" {
		return nil
	}
	name := m.name
	m.name = ""
	if err := shmUnlink(name); err != nil {
		m.name = name
		return err
	}
	return nil
}

// close unmaps, closes the descriptor, and unlinks the name if this handle
// still owns it (failure paths before Ready).
func (m *mapping) close() error {
	var first error
	if err := syscall.Munmap(m.region); err != nil && first == nil {
		first = err
	}
	if err := syscall.Close(m.fd); err != nil && first == nil {
		first = err
	}
	if m.name != "" {
		if err := shmUnlink(m.name); err != nil && first == nil {
			first = err
		}
		m.name = ""
	}
	m.region = nil
	return first
}

func mapShared(fd int, name string, capacity int) (*mapping, error) {
	region, err := syscall.Mmap(fd, 0, ringOffset+capacity, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		syscall.Close(fd)
		if name != "" {
			shmUnlink(name)
		}
		return nil, err
	}
	return &mapping{fd: fd, region: region, name: name, capacity: capacity}, nil
}

// writeMetadata fills the 64-byte immutable header: eight-byte magic,
// big-endian layout version, protocol version, session id, region size,
// ring offset, ring capacity, and record header size. Bytes 40-63 stay zero.
func writeMetadata(header []byte, id uint64, capacity int, protocolVersion uint32) {
	copy(header[0:8], headerMagic[:])
	binary.BigEndian.PutUint32(header[8:12], layoutVersion)
	binary.BigEndian.PutUint32(header[12:16], protocolVersion)
	binary.BigEndian.PutUint64(header[16:24], id)
	binary.BigEndian.PutUint32(header[24:28], uint32(ringOffset+capacity))
	binary.BigEndian.PutUint32(header[28:32], ringOffset)
	binary.BigEndian.PutUint32(header[32:36], uint32(capacity))
	binary.BigEndian.PutUint32(header[36:40], recordHeaderSize)
}

func anyNonZero(b []byte) bool {
	for _, value := range b {
		if value != 0 {
			return true
		}
	}
	return false
}
