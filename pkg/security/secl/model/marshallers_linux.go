// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
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
	if len(data) < 72 {
		return 0, ErrNotEnoughSpace
	}
	ByteOrder.PutUint64(data[0:8], e.Inode)
	ByteOrder.PutUint32(data[8:12], e.MountID)
	ByteOrder.PutUint32(data[12:16], e.PathID)
	ByteOrder.PutUint32(data[16:20], uint32(e.Flags))

	// +4 for padding

	ByteOrder.PutUint32(data[24:28], e.UID)
	ByteOrder.PutUint32(data[28:32], e.GID)
	ByteOrder.PutUint32(data[32:36], e.NLink)
	ByteOrder.PutUint16(data[36:38], e.Mode)

	// +2 for padding

	timeSec := time.Unix(0, int64(e.CTime)).Unix()
	timeNsec := time.Unix(0, int64(e.CTime)).UnixNano()
	timeNsec = timeNsec - timeSec*int64(math.Pow10(9))
	if timeNsec < 0 {
		timeNsec = 0
	}
	ByteOrder.PutUint64(data[40:48], uint64(timeSec))
	ByteOrder.PutUint64(data[48:56], uint64(timeNsec))

	timeSec = time.Unix(0, int64(e.MTime)).Unix()
	timeNsec = time.Unix(0, int64(e.MTime)).UnixNano()
	timeNsec = timeNsec - timeSec*int64(math.Pow10(9))
	if timeNsec < 0 {
		timeNsec = 0
	}
	ByteOrder.PutUint64(data[56:64], uint64(timeSec))
	ByteOrder.PutUint64(data[64:72], uint64(timeNsec))
	return 72, nil
}

// MarshalProcCache marshals a binary representation of itself
func (e *Process) MarshalProcCache(data []byte) (int, error) {
	// Marshal proc_cache_t
	if len(data) < ContainerIDLen {
		return 0, ErrNotEnoughSpace
	}
	copy(data[0:ContainerIDLen], e.ContainerID)
	written := ContainerIDLen

	toAdd, err := MarshalBinary(data[written:], &e.FileEvent)
	if err != nil {
		return 0, err
	}
	written += toAdd

	if len(data[written:]) < 88 {
		return 0, ErrNotEnoughSpace
	}

	marshalTime(data[written:written+8], e.ExecTime)
	written += 8

	copy(data[written:written+64], e.TTYName)
	written += 64

	copy(data[written:written+16], e.Comm)
	written += 16
	return written, nil
}

func marshalTime(data []byte, t time.Time) {
	ByteOrder.PutUint64(data, uint64(t.UnixNano()))
}

// MarshalBinary marshalls a binary representation of itself
func (e *Credentials) MarshalBinary(data []byte) (int, error) {
	if len(data) < 40 {
		return 0, ErrNotEnoughSpace
	}

	ByteOrder.PutUint32(data[0:4], e.UID)
	ByteOrder.PutUint32(data[4:8], e.GID)
	ByteOrder.PutUint32(data[8:12], e.EUID)
	ByteOrder.PutUint32(data[12:16], e.EGID)
	ByteOrder.PutUint32(data[16:20], e.FSUID)
	ByteOrder.PutUint32(data[20:24], e.FSGID)
	ByteOrder.PutUint64(data[24:32], e.CapEffective)
	ByteOrder.PutUint64(data[32:40], e.CapPermitted)
	return 40, nil
}

// MarshalPidCache marshals a binary representation of itself
func (e *Process) MarshalPidCache(data []byte) (int, error) {
	// Marshal pid_cache_t
	if len(data) < 24 {
		return 0, ErrNotEnoughSpace
	}
	ByteOrder.PutUint64(data[0:8], e.Cookie)
	ByteOrder.PutUint32(data[8:12], e.PPid)

	// padding

	marshalTime(data[16:24], e.ForkTime)
	marshalTime(data[24:32], e.ExitTime)
	written := 32

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
	ByteOrder.PutUint64(raw[0:8], eventMask)
	ByteOrder.PutUint64(raw[8:16], uint64(adlc.Timeout))
	ByteOrder.PutUint64(raw[16:24], adlc.WaitListTimestampRaw)
	ByteOrder.PutUint64(raw[24:32], adlc.StartTimestampRaw)
	ByteOrder.PutUint64(raw[32:40], adlc.EndTimestampRaw)
	ByteOrder.PutUint32(raw[40:44], adlc.Rate)
	ByteOrder.PutUint32(raw[44:48], adlc.Paused)

	return raw, nil
}
