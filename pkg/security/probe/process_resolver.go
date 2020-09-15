// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"time"
)

// ProcCacheEntry this structure holds the container context that we keep in kernel for each process
type ProcCacheEntry struct {
	FileEvent
	ContainerEvent
	TimestampRaw uint64
	Timestamp    time.Time
}

// UnmarshalBinary returns the binary representation of itself
func (pc *ProcCacheEntry) UnmarshalBinary(data []byte) (int, error) {
	if len(data) < 45 {
		return 0, ErrNotEnoughData
	}

	read, err := unmarshalBinary(data, &pc.FileEvent, &pc.ContainerEvent)
	if err != nil {
		return 0, err
	}

	pc.TimestampRaw = byteOrder.Uint64(data[read : read+8])

	return read + 8, nil
}

type ProcessResolverEntry struct {
	FileEvent
	ContainerEvent
	Timestamp time.Time
}
