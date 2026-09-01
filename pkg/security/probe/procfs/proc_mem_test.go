// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"math"
	"os"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"
)

func TestMemReadUint64(t *testing.T) {
	var probe uint64 = 0x1122334455667788

	mem, err := OpenMem(uint32(os.Getpid()))
	require.NoError(t, err)
	defer mem.Close()

	got, err := mem.ReadUint64(uint64(uintptr(unsafe.Pointer(&probe))))
	require.NoError(t, err)
	require.Equal(t, probe, got)
}

func TestMemReadUnreadable(t *testing.T) {
	mem, err := OpenMem(uint32(os.Getpid()))
	require.NoError(t, err)
	defer mem.Close()

	// The NULL page is never mapped, so this reaches the kernel and comes back
	// EIO rather than short.
	_, err = mem.ReadUint64(0)
	require.Error(t, err)

	// A kernel address has no pread offset to name it, and is refused before
	// reaching the kernel rather than wrapping around into user space.
	_, err = mem.ReadUint64(math.MaxUint64)
	require.ErrorContains(t, err, "out of the addressable range")
}
