// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Adapted from the Fast IPC Toolkit's lib/go/fitcore (module `fit`): the transport
// comes from commit 4961722de9009afdbbb711fc0adf14a8f6ff9277, and the broadcast
// transport from commit 788233d2ffcc1e9d19b8e8202ca7908b64c64687
// (ddoghq-sandbox/celian-26q4-innov-fast-ipc-toolkit).
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
	fd         int
	region     []byte
	name       string // shm object name while this handle still owns the unlink
	capacity   int
	regionLen  int // total mapped length, including any broadcast slot table
	ringOffset int // physical byte offset of the payload ring inside region
}

// word returns the 32-bit atomic alias at a fixed offset. mmap is
// page-aligned and both fixed offsets meet the four-byte atomic alignment.
func (m *mapping) word(offset int) *uint32 {
	return (*uint32)(unsafe.Add(unsafe.Pointer(unsafe.SliceData(m.region)), offset))
}

// ring returns the raw ring bytes starting at ringOffset.
func (m *mapping) ring() []byte {
	return m.region[m.ringOffset : m.ringOffset+m.capacity]
}

// createMapping creates a fresh SPSC POSIX shared-memory object, maps it, and
// initializes the immutable metadata, both indexes, and the ring to zero
// before the object name is disclosed to the peer.
func createMapping(id uint64, capacity int, protocolVersion uint32) (*mapping, string, error) {
	if err := validateCapacity(capacity); err != nil {
		return nil, "", err
	}
	regionSize := ringOffset + capacity
	region := make([]byte, headerSize)
	writeMetadata(region, id, capacity, protocolVersion)
	shared, name, err := createMappingObject(id, regionSize, ringOffset, capacity, region)
	if err != nil {
		return nil, "", err
	}
	return shared, name, nil
}

// createBroadcastMapping creates the broadcast control region: immutable
// metadata, the write cursor, the subscriber slot table, then the ring. The
// whole region is zeroed before the name is disclosed so a late subscriber
// never sees a partially initialized layout.
func createBroadcastMapping(id uint64, capacity int, protocolVersion uint32, maxSubscribers int) (*mapping, string, error) {
	if err := validateBroadcastLayout(capacity, maxSubscribers); err != nil {
		return nil, "", err
	}
	ringOffsetValue := broadcastSlotsOffset + broadcastSlotStride*maxSubscribers
	regionSize := ringOffsetValue + capacity
	// Only the immutable metadata region participates in validation; the write
	// cursor at broadcastWriteOffset and the slots below are live controls.
	region := make([]byte, broadcastWriteOffset)
	copy(region[broadcastMetaMagic:broadcastMetaMagic+8], broadcastMagic[:])
	binary.BigEndian.PutUint32(region[broadcastMetaLayoutVersion:broadcastMetaLayoutVersion+4], broadcastLayoutVersion)
	binary.BigEndian.PutUint32(region[broadcastMetaProtocolVersion:broadcastMetaProtocolVersion+4], protocolVersion)
	binary.BigEndian.PutUint64(region[broadcastMetaSession:broadcastMetaSession+8], id)
	binary.BigEndian.PutUint32(region[broadcastMetaRegionSize:broadcastMetaRegionSize+4], uint32(regionSize))
	binary.BigEndian.PutUint32(region[broadcastMetaRingOffset:broadcastMetaRingOffset+4], uint32(ringOffsetValue))
	binary.BigEndian.PutUint32(region[broadcastMetaCapacity:broadcastMetaCapacity+4], uint32(capacity))
	binary.BigEndian.PutUint32(region[broadcastMetaRecordHeader:broadcastMetaRecordHeader+4], recordHeaderSize)
	binary.BigEndian.PutUint32(region[broadcastMetaSlotStride:broadcastMetaSlotStride+4], broadcastSlotStride)
	binary.BigEndian.PutUint32(region[broadcastMetaMaxSubscribers:broadcastMetaMaxSubscribers+4], uint32(maxSubscribers))
	binary.BigEndian.PutUint32(region[broadcastMetaSlotsOffset:broadcastMetaSlotsOffset+4], broadcastSlotsOffset)
	return createMappingObject(id, regionSize, ringOffsetValue, capacity, region)
}

// createMappingObject creates and maps a POSIX shared-memory object and copies
// the immutable metadata template into it. The copy is intentionally explicit
// and zeroes everything outside the template, matching the Rust template.
func createMappingObject(id uint64, regionSize, ringOffsetValue, capacity int, metadata []byte) (*mapping, string, error) {
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
	shared, err := mapShared(fd, name, capacity, regionSize, ringOffsetValue)
	if err != nil {
		return nil, "", err
	}
	for index := range shared.region {
		shared.region[index] = 0
	}
	copy(shared.region, metadata)
	return shared, name, nil
}

// openMapping maps the offered SPSC object on the producer side and validates
// its backing size, owner, mode, header, and fresh zeroed queue region.
func openMapping(name string, id uint64, capacity int, protocolVersion uint32) (*mapping, error) {
	if err := validateCapacity(capacity); err != nil {
		return nil, err
	}
	regionSize := ringOffset + capacity
	expected := make([]byte, headerSize)
	writeMetadata(expected, id, capacity, protocolVersion)
	shared, err := openMappingObject(name, regionSize, ringOffset, capacity, expected)
	if err != nil {
		return nil, err
	}
	if anyNonZero(shared.region[headerSize:]) {
		shared.close()
		return nil, invalid("shared-memory header or fresh queue region is invalid")
	}
	return shared, nil
}

// openBroadcastMapping maps a live broadcast object for a late subscriber. Only
// the immutable metadata is validated; the fresh-zero check used for SPSC must
// not be applied here: subscribers join a changing mapping.
func openBroadcastMapping(name string, id uint64, capacity int, protocolVersion uint32, maxSubscribers, expectedRegionSize, expectedRingOffset int) (*mapping, error) {
	if err := validateBroadcastLayout(capacity, maxSubscribers); err != nil {
		return nil, err
	}
	ringOffsetValue := broadcastSlotsOffset + broadcastSlotStride*maxSubscribers
	regionSize := ringOffsetValue + capacity
	if regionSize != expectedRegionSize || ringOffsetValue != expectedRingOffset {
		return nil, invalid("broadcast offered bounds are inconsistent")
	}
	expected := make([]byte, broadcastWriteOffset)
	copy(expected[broadcastMetaMagic:broadcastMetaMagic+8], broadcastMagic[:])
	binary.BigEndian.PutUint32(expected[broadcastMetaLayoutVersion:broadcastMetaLayoutVersion+4], broadcastLayoutVersion)
	binary.BigEndian.PutUint32(expected[broadcastMetaProtocolVersion:broadcastMetaProtocolVersion+4], protocolVersion)
	binary.BigEndian.PutUint64(expected[broadcastMetaSession:broadcastMetaSession+8], id)
	binary.BigEndian.PutUint32(expected[broadcastMetaRegionSize:broadcastMetaRegionSize+4], uint32(regionSize))
	binary.BigEndian.PutUint32(expected[broadcastMetaRingOffset:broadcastMetaRingOffset+4], uint32(ringOffsetValue))
	binary.BigEndian.PutUint32(expected[broadcastMetaCapacity:broadcastMetaCapacity+4], uint32(capacity))
	binary.BigEndian.PutUint32(expected[broadcastMetaRecordHeader:broadcastMetaRecordHeader+4], recordHeaderSize)
	binary.BigEndian.PutUint32(expected[broadcastMetaSlotStride:broadcastMetaSlotStride+4], broadcastSlotStride)
	binary.BigEndian.PutUint32(expected[broadcastMetaMaxSubscribers:broadcastMetaMaxSubscribers+4], uint32(maxSubscribers))
	binary.BigEndian.PutUint32(expected[broadcastMetaSlotsOffset:broadcastMetaSlotsOffset+4], broadcastSlotsOffset)
	return openMappingObject(name, regionSize, ringOffsetValue, capacity, expected)
}

// openMappingObject opens, validates, and maps an existing POSIX shared-memory
// object whose first len(expected) bytes must equal expected.
func openMappingObject(name string, regionSize, ringOffsetValue, capacity int, expected []byte) (*mapping, error) {
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
	shared, err := mapShared(fd, "", capacity, regionSize, ringOffsetValue)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(shared.region[:len(expected)], expected) {
		shared.close()
		return nil, invalid("shared-memory header mismatch")
	}
	return shared, nil
}

// unlinkName removes the creator-owned shm object name after the peer mapped
// it; both peers keep their mapping for their local session.
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
// still owns it (failure paths before the peer mapped the object).
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

func mapShared(fd int, name string, capacity, regionSize, ringOffsetValue int) (*mapping, error) {
	region, err := syscall.Mmap(fd, 0, regionSize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		syscall.Close(fd)
		if name != "" {
			shmUnlink(name)
		}
		return nil, err
	}
	return &mapping{fd: fd, region: region, name: name, capacity: capacity, regionLen: regionSize, ringOffset: ringOffsetValue}, nil
}

// writeMetadata fills the 64-byte immutable SPSC header: eight-byte magic,
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
