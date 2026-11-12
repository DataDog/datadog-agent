// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package meminfo

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// sampleMeminfo is a representative modern-kernel /proc/meminfo excerpt
// (fields free actually consumes, plus HugePages_Total as a
// representative unitless field that must be skipped).
const sampleMeminfo = `MemTotal:       16332188 kB
MemFree:         8974932 kB
MemAvailable:   14000000 kB
Buffers:          200000 kB
Cached:          5987768 kB
SwapCached:            0 kB
Active:          4000000 kB
Inactive:        3000000 kB
SReclaimable:     100000 kB
SUnreclaim:        50000 kB
Shmem:            123456 kB
SwapTotal:       2097148 kB
SwapFree:        2097148 kB
HugePages_Total:       0
`

func TestParseMeminfo_HappyPath(t *testing.T) {
	info, err := parseMeminfo(context.Background(), strings.NewReader(sampleMeminfo))
	assert.NoError(t, err)
	assert.Equal(t, uint64(16332188*1024), info.MemTotal)
	assert.Equal(t, uint64(8974932*1024), info.MemFree)
	assert.Equal(t, uint64(14000000*1024), info.MemAvailable, "MemAvailable is read directly from the kernel when present")
	assert.Equal(t, uint64(200000*1024), info.Buffers)
	assert.Equal(t, uint64(5987768*1024), info.Cached)
	assert.Equal(t, uint64(100000*1024), info.SReclaimable)
	assert.Equal(t, uint64(123456*1024), info.Shared)
	assert.Equal(t, uint64(2097148*1024), info.SwapTotal)
	assert.Equal(t, uint64(2097148*1024), info.SwapFree)
}

func TestParseMeminfo_MemAvailableFallback(t *testing.T) {
	// Pre-3.14 kernels do not report MemAvailable; readImpl approximates
	// it as MemFree+Buffers+Cached.
	input := `MemTotal:       16332188 kB
MemFree:         8000000 kB
Buffers:          200000 kB
Cached:          5000000 kB
`
	info, err := parseMeminfo(context.Background(), strings.NewReader(input))
	assert.NoError(t, err)
	want := uint64(8000000+200000+5000000) * 1024
	assert.Equal(t, want, info.MemAvailable)
}

func TestParseMeminfo_SkipsUnitlessAndMalformedLines(t *testing.T) {
	input := `MemTotal:       16332188 kB
HugePages_Total:       0
no colon here
Malformed: not a number kB
MemFree:         8000000 kB
`
	info, err := parseMeminfo(context.Background(), strings.NewReader(input))
	assert.NoError(t, err)
	assert.Equal(t, uint64(16332188*1024), info.MemTotal)
	assert.Equal(t, uint64(8000000*1024), info.MemFree)
}

func TestParseMeminfo_EmptyInput(t *testing.T) {
	info, err := parseMeminfo(context.Background(), strings.NewReader(""))
	assert.NoError(t, err)
	assert.Equal(t, Info{}, info)
}

func TestParseMeminfoLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		wantKey string
		wantKiB uint64
		wantOK  bool
	}{
		{"basic", "MemTotal:       16332188 kB", "MemTotal", 16332188, true},
		{"tight spacing", "MemFree:1 kB", "MemFree", 1, true},
		{"zero", "SwapFree:        0 kB", "SwapFree", 0, true},
		{"no colon", "MemTotal 16332188 kB", "", 0, false},
		{"unitless", "HugePages_Total:       0", "", 0, false},
		{"non-numeric value", "MemTotal:       abc kB", "", 0, false},
		{"empty value", "MemTotal: kB", "", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, kib, ok := parseMeminfoLine(tt.line)
			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantKey, key)
				assert.Equal(t, tt.wantKiB, kib)
			}
		})
	}
}

func TestMulSat_Overflow(t *testing.T) {
	assert.Equal(t, ^uint64(0), mulSat(^uint64(0), 2))
	assert.Equal(t, uint64(0), mulSat(0, 1024))
	assert.Equal(t, uint64(2048), mulSat(2, 1024))
}

func TestParseMeminfo_LineTooLong(t *testing.T) {
	// A single line far exceeding maxMeminfoLine must not crash the
	// scanner; bufio.Scanner surfaces bufio.ErrTooLong via scanner.Err(),
	// which parseMeminfo propagates.
	huge := "MemTotal:" + strings.Repeat("0", maxMeminfoLine*2) + " kB\n"
	_, err := parseMeminfo(context.Background(), strings.NewReader(huge))
	assert.Error(t, err)
}

func TestParseMeminfo_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := parseMeminfo(ctx, strings.NewReader(sampleMeminfo))
	assert.ErrorIs(t, err, context.Canceled)
}
