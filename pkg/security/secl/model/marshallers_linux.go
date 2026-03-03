// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"encoding/binary"
	"math"
	"time"
)

// BinaryMarshaler interface implemented by every event type
type BinaryMarshaler interface {
	MarshalBinary(data []byte) (int, error)
}

// MarshalBinary calls a series of BinaryMarshaler
func MarshalBinary(data []byte, binaryMarshalers ...BinaryMarshaler) (int, error) {
	written := 0
	for _, marshaler := range binaryMarshalers {
		n, err := marshaler.MarshalBinary(data[written:])
		written += n
		if err != nil {
			return written, err
		}
	}
	return written, nil
}

// MarshalBinary marshals a binary representation of itself
func (e *FileFields) MarshalBinary(data []byte) (int, error) {
	if len(data) < FileFieldsSize {
		return 0, ErrNotEnoughSpace
	}
	binary.NativeEndian.PutUint64(data[0:8], e.Inode)
	binary.NativeEndian.PutUint32(data[8:12], e.MountID)
	binary.NativeEndian.PutUint32(data[12:16], e.PathID)
	binary.NativeEndian.PutUint32(data[16:20], uint32(e.Flags))

	// +4 for padding

	binary.NativeEndian.PutUint32(data[24:28], e.UID)
	binary.NativeEndian.PutUint32(data[28:32], e.GID)
	binary.NativeEndian.PutUint32(data[32:36], e.NLink)
	binary.NativeEndian.PutUint16(data[36:38], e.Mode)

	// +2 for padding

	timeSec := time.Unix(0, int64(e.CTime)).Unix()
	timeNsec := time.Unix(0, int64(e.CTime)).UnixNano()
	timeNsec = timeNsec - timeSec*int64(math.Pow10(9))
	if timeNsec < 0 {
		timeNsec = 0
	}
	binary.NativeEndian.PutUint64(data[40:48], uint64(timeSec))
	binary.NativeEndian.PutUint64(data[48:56], uint64(timeNsec))

	timeSec = time.Unix(0, int64(e.MTime)).Unix()
	timeNsec = time.Unix(0, int64(e.MTime)).UnixNano()
	timeNsec = timeNsec - timeSec*int64(math.Pow10(9))
	if timeNsec < 0 {
		timeNsec = 0
	}
	binary.NativeEndian.PutUint64(data[56:64], uint64(timeSec))
	binary.NativeEndian.PutUint64(data[64:72], uint64(timeNsec))

	return FileFieldsSize, nil
}

// MarshalProcCache marshals a binary representation of itself
func (e *Process) MarshalProcCache(data []byte, bootTime time.Time) (int, error) {
	// Marshal proc_cache_t

	// marshal cgroup_context/path_key of size 16
	if len(data) < PathKeySize {
		return 0, ErrNotEnoughSpace
	}
	e.CGroup.CGroupPathKey.Write(data)
	written := PathKeySize

	// marshal file_t executable of size 72
	if len(data[written:]) < 72 {
		return 0, ErrNotEnoughSpace
	}
	added, err := MarshalBinary(data[written:], &e.FileEvent)
	if err != nil {
		return 0, err
	}
	written += added

	// marshal exec_timestamp / tty_name / comm, total size of 88 (8 + 64 + 16)
	if len(data[written:]) < 88 {
		return 0, ErrNotEnoughSpace
	}

	marshalTime(data[written:written+8], e.ExecTime.Sub(bootTime))
	written += 8

	copy(data[written:written+64], e.TTYName)
	written += 64

	copy(data[written:written+16], e.Comm)
	written += 16
	return written, nil
}

func marshalTime(data []byte, t time.Duration) {
	binary.NativeEndian.PutUint64(data, uint64(t.Nanoseconds()))
}

// MarshalBinary marshalls a binary representation of itself
func (e *Credentials) MarshalBinary(data []byte) (int, error) {
	if len(data) < 48 {
		return 0, ErrNotEnoughSpace
	}

	binary.NativeEndian.PutUint32(data[0:4], e.UID)
	binary.NativeEndian.PutUint32(data[4:8], e.GID)
	binary.NativeEndian.PutUint32(data[8:12], e.EUID)
	binary.NativeEndian.PutUint32(data[12:16], e.EGID)
	binary.NativeEndian.PutUint32(data[16:20], e.FSUID)
	binary.NativeEndian.PutUint32(data[20:24], e.FSGID)
	binary.NativeEndian.PutUint32(data[24:28], e.AUID)
	binary.NativeEndian.PutUint32(data[28:32], 1)
	binary.NativeEndian.PutUint64(data[32:40], e.CapEffective)
	binary.NativeEndian.PutUint64(data[40:48], e.CapPermitted)
	return 48, nil
}

// MarshalPidCache marshals a binary representation of itself
func (e *Process) MarshalPidCache(data []byte, bootTime time.Time) (int, error) {
	// Marshal pid_cache_t
	if len(data) < 88 {
		return 0, ErrNotEnoughSpace
	}
	binary.NativeEndian.PutUint64(data[0:8], e.Cookie)
	binary.NativeEndian.PutUint32(data[8:12], e.PPid)

	// padding

	marshalTime(data[16:24], e.ForkTime.Sub(bootTime))
	marshalTime(data[24:32], e.ExitTime.Sub(bootTime))
	binary.NativeEndian.PutUint64(data[32:40], e.UserSession.K8SSessionID)
	written := 40

	n, err := MarshalBinary(data[written:], &e.Credentials)
	if err != nil {
		return 0, err
	}
	written += n

	return written, nil
}

// MarshalBinary marshals a binary representation of itself
func (adlc *ActivityDumpLoadConfig) MarshalBinary() ([]byte, error) {
	raw := make([]byte, 48)

	var eventMask uint64
	for _, evt := range adlc.TracedEventTypes {
		eventMask |= 1 << (evt - FirstDiscarderEventType)
	}
	binary.NativeEndian.PutUint64(raw[0:8], eventMask)
	binary.NativeEndian.PutUint64(raw[8:16], uint64(adlc.Timeout))
	binary.NativeEndian.PutUint64(raw[16:24], adlc.WaitListTimestampRaw)
	binary.NativeEndian.PutUint64(raw[24:32], adlc.StartTimestampRaw)
	binary.NativeEndian.PutUint64(raw[32:40], adlc.EndTimestampRaw)
	binary.NativeEndian.PutUint16(raw[40:42], adlc.Rate)
	binary.NativeEndian.PutUint16(raw[42:44], 0)
	binary.NativeEndian.PutUint32(raw[44:48], adlc.Paused)

	return raw, nil
}

// MarshalBinary returns the binary representation of a path key
func (pl *PathLeaf) MarshalBinary() ([]byte, error) {
	buff := make([]byte, PathLeafSize)

	pl.Parent.Write(buff)
	copy(buff[16:], pl.Name[:])
	binary.NativeEndian.PutUint16(buff[16+len(pl.Name):], pl.Len)

	return buff, nil
}

func (p *PathKey) Write(buffer []byte) {
	binary.NativeEndian.PutUint64(buffer[0:8], p.Inode)
	binary.NativeEndian.PutUint32(buffer[8:12], p.MountID)
	binary.NativeEndian.PutUint32(buffer[12:16], p.PathID)
}

// MarshalBinary returns the binary representation of a path key
func (p *PathKey) MarshalBinary() ([]byte, error) {
	if p.IsNull() {
		return nil, &ErrInvalidKeyPath{Inode: p.Inode, MountID: p.MountID}
	}

	buff := make([]byte, PathKeySize)
	p.Write(buff)

	return buff, nil
}
