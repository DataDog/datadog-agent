// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package model

import (
	"bytes"
	"time"
	"unsafe"

	"github.com/pkg/errors"
)

// ErrNotEnoughData is returned when the buffer is too small to unmarshal the event
var ErrNotEnoughData = errors.New("not enough data")

// BinaryUnmarshaler interface implemented by every event type
type BinaryUnmarshaler interface {
	UnmarshalBinary(data []byte) (int, error)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChmodEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ChownEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.UID = ByteOrder.Uint32(data[0:4])
	e.GID = ByteOrder.Uint32(data[4:8])
	return n + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ContainerContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 64 {
		return 0, ErrNotEnoughData
	}

	idRaw := [64]byte{}
	SliceToArray(data[0:64], unsafe.Pointer(&idRaw))
	e.ID = string(bytes.Trim(idRaw[:], "\x00"))
	if len(e.ID) > 1 && len(e.ID) < 64 {
		e.ID = ""
	}

	return 64, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *Event) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 24 {
		return 0, ErrNotEnoughData
	}

	e.TimestampRaw = ByteOrder.Uint64(data[8:16])
	e.Type = ByteOrder.Uint64(data[16:24])

	return 24, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SetuidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.UID = ByteOrder.Uint32(data[0:4])
	e.EUID = ByteOrder.Uint32(data[4:8])
	e.FSUID = ByteOrder.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SetgidEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.GID = ByteOrder.Uint32(data[0:4])
	e.EGID = ByteOrder.Uint32(data[4:8])
	e.FSGID = ByteOrder.Uint32(data[8:12])
	return 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *CapsetEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}
	e.CapEffective = ByteOrder.Uint64(data[0:8])
	e.CapPermitted = ByteOrder.Uint64(data[8:16])
	return 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *Credentials) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 40 {
		return 0, ErrNotEnoughData
	}

	e.UID = ByteOrder.Uint32(data[0:4])
	e.GID = ByteOrder.Uint32(data[4:8])
	e.EUID = ByteOrder.Uint32(data[8:12])
	e.EGID = ByteOrder.Uint32(data[12:16])
	e.FSUID = ByteOrder.Uint32(data[16:20])
	e.FSGID = ByteOrder.Uint32(data[20:24])
	e.CapEffective = ByteOrder.Uint64(data[24:32])
	e.CapPermitted = ByteOrder.Uint64(data[32:40])
	return 40, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *ExecEvent) UnmarshalBinary(data []byte) (int, error) {
	// Unmarshal proc_cache_t
	read, err := UnmarshalBinary(data, &e.FileFields)
	if err != nil {
		return 0, err
	}

	if len(data[read:]) < 112 {
		return 0, ErrNotEnoughData
	}

	e.ExecTimestamp = ByteOrder.Uint64(data[read : read+8])
	read += 8

	var ttyRaw [64]byte
	SliceToArray(data[read:read+64], unsafe.Pointer(&ttyRaw))
	e.TTYName = string(bytes.Trim(ttyRaw[:], "\x00"))
	read += 64

	var commRaw [16]byte
	SliceToArray(data[read:read+16], unsafe.Pointer(&commRaw))
	e.Comm = string(bytes.Trim(commRaw[:], "\x00"))
	read += 16

	// Unmarshal pid_cache_t
	e.Cookie = ByteOrder.Uint32(data[read : read+4])
	e.PPid = ByteOrder.Uint32(data[read+4 : read+8])

	e.ForkTimestamp = ByteOrder.Uint64(data[read+8 : read+16])
	e.ExitTimestamp = ByteOrder.Uint64(data[read+16 : read+24])
	read += 24

	// Unmarshal the credentials contained in pid_cache_t
	return UnmarshalBinary(data[read:], &e.Credentials)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *InvalidateDentryEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 16 {
		return 0, ErrNotEnoughData
	}

	e.Inode = ByteOrder.Uint64(data[0:8])
	e.MountID = ByteOrder.Uint32(data[8:12])
	e.DiscarderRevision = ByteOrder.Uint32(data[12:16])

	return 16, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *FileFields) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 72 {
		return 0, ErrNotEnoughData
	}
	e.Inode = ByteOrder.Uint64(data[0:8])
	e.MountID = ByteOrder.Uint32(data[8:12])
	e.OverlayNumLower = int32(ByteOrder.Uint32(data[12:16]))
	e.PathID = ByteOrder.Uint32(data[16:20])

	// +4 for padding

	e.UID = ByteOrder.Uint32(data[24:28])
	e.GID = ByteOrder.Uint32(data[28:32])
	e.Mode = ByteOrder.Uint16(data[32:34])

	// +6 for padding

	timeSec := ByteOrder.Uint64(data[40:48])
	timeNsec := ByteOrder.Uint64(data[48:56])
	e.CTime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = ByteOrder.Uint64(data[56:64])
	timeNsec = ByteOrder.Uint64(data[64:72])
	e.MTime = time.Unix(int64(timeSec), int64(timeNsec))
	return 72, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *FileEvent) UnmarshalBinary(data []byte) (int, error) {
	return e.FileFields.UnmarshalBinary(data)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *LinkEvent) UnmarshalBinary(data []byte) (int, error) {
	return UnmarshalBinary(data, &e.SyscallEvent, &e.Source, &e.Target)
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MkdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 4 {
		return n, ErrNotEnoughData
	}

	e.Mode = ByteOrder.Uint32(data[0:4])
	return n + 4, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *MountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 56 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ByteOrder.Uint32(data[0:4])
	e.GroupID = ByteOrder.Uint32(data[4:8])
	e.Device = ByteOrder.Uint32(data[8:12])
	e.ParentMountID = ByteOrder.Uint32(data[12:16])
	e.ParentInode = ByteOrder.Uint64(data[16:24])
	e.RootInode = ByteOrder.Uint64(data[24:32])
	e.RootMountID = ByteOrder.Uint32(data[32:36])

	// Notes: bytes 36 to 40 are used to pad the structure

	SliceToArray(data[40:56], unsafe.Pointer(&e.FSTypeRaw))

	return 56, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *OpenEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return n, ErrNotEnoughData
	}

	e.Flags = ByteOrder.Uint32(data[0:4])
	e.Mode = ByteOrder.Uint32(data[4:8])
	return n + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (p *ProcessContext) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	p.Pid = ByteOrder.Uint32(data[0:4])
	p.Tid = ByteOrder.Uint32(data[4:8])

	return 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RenameEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.Old, &e.New)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.DiscarderRevision = ByteOrder.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *RmdirEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.DiscarderRevision = ByteOrder.Uint32(data[0:4])
	// padding

	return n + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SetXAttrEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 200 {
		return n, ErrNotEnoughData
	}
	SliceToArray(data[0:200], unsafe.Pointer(&e.NameRaw))

	return n + 200, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *SyscallEvent) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}
	e.Retval = int64(ByteOrder.Uint64(data[0:8]))
	return 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UmountEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.MountID = ByteOrder.Uint32(data[0:4])
	e.DiscarderRevision = ByteOrder.Uint32(data[4:8])

	return 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UnlinkEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 8 {
		return 0, ErrNotEnoughData
	}

	e.Flags = ByteOrder.Uint32(data[0:4])
	e.DiscarderRevision = ByteOrder.Uint32(data[4:8])

	return n + 8, nil
}

// UnmarshalBinary unmarshals a binary representation of itself
func (e *UtimesEvent) UnmarshalBinary(data []byte) (int, error) {
	n, err := UnmarshalBinary(data, &e.SyscallEvent, &e.File)
	if err != nil {
		return n, err
	}

	data = data[n:]
	if len(data) < 32 {
		return 0, ErrNotEnoughData
	}

	timeSec := ByteOrder.Uint64(data[0:8])
	timeNsec := ByteOrder.Uint64(data[8:16])
	e.Atime = time.Unix(int64(timeSec), int64(timeNsec))

	timeSec = ByteOrder.Uint64(data[16:24])
	timeNsec = ByteOrder.Uint64(data[24:32])
	e.Mtime = time.Unix(int64(timeSec), int64(timeNsec))

	return n + 32, nil
}

// UnmarshalBinary calls a series of BinaryUnmarshaler
func UnmarshalBinary(data []byte, binaryUnmarshalers ...BinaryUnmarshaler) (int, error) {
	read := 0
	for _, marshaler := range binaryUnmarshalers {
		n, err := marshaler.UnmarshalBinary(data[read:])
		read += n
		if err != nil {
			return read, err
		}
	}
	return read, nil
}
