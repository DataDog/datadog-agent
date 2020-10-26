// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"bytes"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/pkg/security/ebpf"
	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// ProcessCacheEntry this structure holds the container context that we keep in kernel for each process
type ProcessCacheEntry struct {
	FileEvent
	ContainerEvent
	TimestampRaw uint64
	Timestamp    time.Time
	Cookie       uint32
	TTYName      string
	Comm         string

	TTYNameRaw [64]byte
}

// UnmarshalBinary returns the binary representation of itself
func (pc *ProcessCacheEntry) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 96 {
		return 0, ErrNotEnoughData
	}

	read, err := unmarshalBinary(data, &pc.FileEvent, &pc.ContainerEvent)
	if err != nil {
		return 0, err
	}

	pc.TimestampRaw = ebpf.ByteOrder.Uint64(data[read : read+8])
	pc.Cookie = ebpf.ByteOrder.Uint32(data[read+8 : read+12])

	// skip 4 for padding
	utils.SliceToArray(data[read+16:read+80], unsafe.Pointer(&pc.TTYNameRaw))

	return read + 80, nil
}

// GetTTY returns the TTY
func (pc *ProcessCacheEntry) GetTTY() string {
	if len(pc.TTYName) == 0 {
		pc.TTYName = string(bytes.Trim(pc.TTYNameRaw[:], "\x00"))
	}
	return pc.TTYName
}
