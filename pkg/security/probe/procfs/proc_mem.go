// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procfs

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// Mem is the memory of a running process, addressed by virtual address.
type Mem struct {
	f *os.File
}

// OpenMem opens the memory of pid for reading.
func OpenMem(pid uint32) (Mem, error) {
	f, err := os.Open(kernel.HostProc(strconv.FormatUint(uint64(pid), 10), "mem"))
	if err != nil {
		return Mem{}, err
	}
	return Mem{f: f}, nil
}

// Close releases the target's memory.
func (m Mem) Close() error {
	return m.f.Close()
}

// Read fills buf from addr, and reports a read it could not satisfy in full as
// an error.
func (m Mem) Read(addr uint64, buf []byte) error {
	if addr > math.MaxInt64 {
		return fmt.Errorf("address %#x is out of the addressable range", addr)
	}
	if _, err := m.f.ReadAt(buf, int64(addr)); err != nil {
		return fmt.Errorf("read %d bytes at %#x: %w", len(buf), addr, err)
	}
	return nil
}

// ReadUint64 reads one 8-byte native-endian word at addr.
func (m Mem) ReadUint64(addr uint64) (uint64, error) {
	var buf [8]byte
	if err := m.Read(addr, buf[:]); err != nil {
		return 0, err
	}
	return binary.NativeEndian.Uint64(buf[:]), nil
}
