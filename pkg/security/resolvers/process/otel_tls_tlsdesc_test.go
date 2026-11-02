// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package process

import (
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeMem is a sparse address space for classifyOTelTLSDesc unit tests.
type fakeMem map[uint64]uint64

func (m fakeMem) ReadUint64(addr uint64) (uint64, error) {
	v, ok := m[addr]
	if !ok {
		return 0, fmt.Errorf("unmapped %#x", addr)
	}
	return v, nil
}

func (m fakeMem) Read(addr uint64, buf []byte) error {
	for i := 0; i+8 <= len(buf); i += 8 {
		v, err := m.ReadUint64(addr + uint64(i))
		if err != nil {
			return err
		}
		binary.NativeEndian.PutUint64(buf[i:i+8], v)
	}
	if rem := len(buf) % 8; rem != 0 {
		return fmt.Errorf("fakeMem.Read: trailing %d bytes at %#x unsupported", rem, addr+uint64(len(buf)-rem))
	}
	return nil
}

func TestClassifyOTelTLSDescAmd64(t *testing.T) {
	if runtime.GOARCH != "amd64" {
		t.Skip("amd64-only discriminant")
	}

	t.Run("negative_arg_is_static", func(t *testing.T) {
		static, _, _, err := classifyOTelTLSDesc(fakeMem{}, 0x1000, ^uint64(0x37)) // -0x38
		require.NoError(t, err)
		require.True(t, static)
	})

	// The exact shape that broke dlopen-glibc-big on KMT: a heap-resident
	// tls_index pointer below 4GiB, which the old `> 0xffffffff` cut took as
	// static.
	t.Run("heap_tls_index_below_4gib_is_dynamic", func(t *testing.T) {
		const arg = 0x32344a80
		mem := fakeMem{
			arg:     2, // module ID
			arg + 8: 0, // in-module offset
		}
		static, moduleID, tlsOffset, err := classifyOTelTLSDesc(mem, 0x7c94d047c780, arg)
		require.NoError(t, err)
		require.False(t, static, "heap pointer must not be classified static")
		require.Equal(t, uint64(2), moduleID)
		require.Equal(t, uint64(0), tlsOffset)
	})

	t.Run("implausible_module_id", func(t *testing.T) {
		const arg = 0x100000
		mem := fakeMem{arg: 0, arg + 8: 0}
		_, _, _, err := classifyOTelTLSDesc(mem, 0x1000, arg)
		require.Error(t, err)
	})
}

func TestClassifyOTelTLSDescArm64(t *testing.T) {
	if runtime.GOARCH != "arm64" {
		t.Skip("arm64-only discriminant")
	}

	t.Run("below_min_user_addr_is_static", func(t *testing.T) {
		static, _, _, err := classifyOTelTLSDesc(fakeMem{}, 0x1000, 0x38)
		require.NoError(t, err)
		require.True(t, static)
	})

	t.Run("static_resolver_body_is_static", func(t *testing.T) {
		const resolver = 0xaaaa0000
		mem := fakeMem{}
		// ldr x0, [x0, #8]; ret — little-endian words as uint64 pairs of
		// half-words stuffed via Read on 4-byte granularity is awkward with
		// fakeMem's 8-byte cells, so pack both instructions into one cell and
		// pad a second zero cell the reader will also touch.
		// 2-insn body + 2 landing-pad slots = 16 bytes (see tlsdesc_arm64).
		var code [16]byte
		binary.LittleEndian.PutUint32(code[0:4], 0xf9400400)
		binary.LittleEndian.PutUint32(code[4:8], 0xd65f03c0)
		for i := 0; i < len(code); i += 8 {
			mem[resolver+uint64(i)] = binary.LittleEndian.Uint64(code[i : i+8])
		}
		static, _, _, err := classifyOTelTLSDesc(mem, resolver, 0x20000)
		require.NoError(t, err)
		require.True(t, static)
	})

	t.Run("non_static_resolver_with_tls_index_is_dynamic", func(t *testing.T) {
		const resolver = 0xaaaa0000
		const arg = 0xbbbb0000
		mem := fakeMem{
			// Unrecognisable resolver body.
			resolver:     0xdeadbeefdeadbeef,
			resolver + 8: 0,
			arg:          3,
			arg + 8:      0x10,
		}
		static, moduleID, tlsOffset, err := classifyOTelTLSDesc(mem, resolver, arg)
		require.NoError(t, err)
		require.False(t, static)
		require.Equal(t, uint64(3), moduleID)
		require.Equal(t, uint64(0x10), tlsOffset)
	})
}
