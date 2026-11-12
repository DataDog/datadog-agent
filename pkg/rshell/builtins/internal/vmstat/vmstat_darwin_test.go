// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package vmstat

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadImpl_Darwin_HappyPath(t *testing.T) {
	st, err := Read(context.Background(), "")
	require.NoError(t, err)

	assert.NotZero(t, st.Partial&FieldMemory, "macOS should always report FieldMemory via hw.memsize")
	assert.NotZero(t, st.MemTotal, "hw.memsize should be non-zero on any real Mac")
	assert.NotZero(t, st.Partial&FieldLoadAvg, "macOS should always report FieldLoadAvg via vm.loadavg")
	assert.GreaterOrEqual(t, st.LoadAvg1, 0.0)
	if st.Partial&FieldSwap != 0 {
		assert.GreaterOrEqual(t, st.SwapTotal, st.SwapFree)
	}

	// Groups with no sysctl backing on macOS must stay unset — a caller
	// that ignores Partial and prints these as "0" would be lying about
	// actual host state.
	assert.Zero(t, st.Partial&FieldProcs)
	assert.Zero(t, st.Partial&FieldPaging)
	assert.Zero(t, st.Partial&FieldSystem)
	assert.Zero(t, st.Partial&FieldCPU)
	assert.Zero(t, st.Partial&FieldMemoryDetail, "macOS has no sysctl for the free/buffers/cached/active/inactive breakdown without Mach host_statistics64")
}

func TestReadU32LE(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03, 0x04}
	assert.EqualValues(t, 0x04030201, readU32LE(data, 0))
	assert.EqualValues(t, 0, readU32LE(data, 1), "out-of-range read returns 0, not a panic")
	assert.EqualValues(t, 0, readU32LE(data, 4))
	assert.EqualValues(t, 0, readU32LE(nil, 0))
}

func TestReadU64LE(t *testing.T) {
	data := make([]byte, 8)
	data[0] = 0xFF
	data[7] = 0x01
	assert.EqualValues(t, 0x01000000000000FF, readU64LE(data, 0))
	assert.EqualValues(t, 0, readU64LE(data, 1))
	assert.EqualValues(t, 0, readU64LE(nil, 0))
}

func TestReadSwap_SyntheticBuffer(t *testing.T) {
	// struct xsw_usage { u_int64 total; u_int64 avail; u_int64 used; u_int32 pagesize; boolean_t encrypted; }
	buf := make([]byte, xswUsageSize)
	putU64LE(buf, 0, 8_000_000_000)  // total
	putU64LE(buf, 8, 7_000_000_000)  // avail
	putU64LE(buf, 16, 1_000_000_000) // used

	var st Stats
	ok := decodeSwapUsage(buf, &st)
	require.True(t, ok)
	assert.EqualValues(t, 8_000_000_000, st.SwapTotal)
	assert.EqualValues(t, 7_000_000_000, st.SwapFree)
}

func TestReadSwap_TooShort(t *testing.T) {
	var st Stats
	ok := decodeSwapUsage(make([]byte, xswUsageSize-1), &st)
	assert.False(t, ok)
}

func TestReadSwap_UsedExceedsTotal(t *testing.T) {
	// A malformed/adversarial sysctl reply where used > total must not
	// underflow SwapFree.
	buf := make([]byte, xswUsageSize)
	putU64LE(buf, 0, 100)
	putU64LE(buf, 16, 200)

	var st Stats
	ok := decodeSwapUsage(buf, &st)
	assert.False(t, ok, "used > total must reject the malformed reply")
	assert.Zero(t, st.SwapTotal)
	assert.Zero(t, st.SwapFree)
}

func TestReadSwap_AvailableExceedsTotal(t *testing.T) {
	buf := make([]byte, xswUsageSize)
	putU64LE(buf, 0, 100)
	putU64LE(buf, 8, 200)

	var st Stats
	assert.False(t, decodeSwapUsage(buf, &st))
	assert.Zero(t, st.SwapTotal)
	assert.Zero(t, st.SwapFree)
}

func TestReadImpl_Darwin_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := readImpl(ctx, "")
	assert.ErrorIs(t, err, context.Canceled)
}

func TestReadLoadAvg_SyntheticBuffer(t *testing.T) {
	buf := make([]byte, loadavgStructSize)
	// fscale = 2048 (LSCALE on darwin); ldavg values are fixed-point scaled.
	putU32LE(buf, 0, 246)  // 246/2048 ≈ 0.12
	putU32LE(buf, 4, 697)  // ≈ 0.34
	putU32LE(buf, 8, 1147) // ≈ 0.56
	putU64LE(buf, 16, 2048)

	var st Stats
	ok := decodeLoadAvg(buf, &st)
	require.True(t, ok)
	assert.InDelta(t, 0.12, st.LoadAvg1, 0.001)
	assert.InDelta(t, 0.34, st.LoadAvg5, 0.001)
	assert.InDelta(t, 0.56, st.LoadAvg15, 0.001)
}

func TestReadLoadAvg_ZeroFscale(t *testing.T) {
	buf := make([]byte, loadavgStructSize) // fscale defaults to 0
	var st Stats
	ok := decodeLoadAvg(buf, &st)
	assert.False(t, ok, "fscale=0 must be rejected, not divide-by-zero")
}

func TestReadLoadAvg_TooShort(t *testing.T) {
	var st Stats
	ok := decodeLoadAvg(make([]byte, loadavgStructSize-1), &st)
	assert.False(t, ok)
}

// putU32LE / putU64LE are the test-side mirror of readU32LE / readU64LE, used
// to build synthetic sysctl-shaped buffers without depending on encoding/binary.
func putU32LE(buf []byte, off int, v uint32) {
	buf[off] = byte(v)
	buf[off+1] = byte(v >> 8)
	buf[off+2] = byte(v >> 16)
	buf[off+3] = byte(v >> 24)
}

func putU64LE(buf []byte, off int, v uint64) {
	putU32LE(buf, off, uint32(v))
	putU32LE(buf, off+4, uint32(v>>32))
}
