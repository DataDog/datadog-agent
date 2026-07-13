// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package pclntab

import (
	"encoding/binary"
	"strings"
	"testing"
)

// moduleDataOf builds a fake moduledata from a list of pointer-sized words.
func moduleDataOf(words ...uint64) []byte {
	b := make([]byte, len(words)*ptrSize)
	for i, w := range words {
		binary.NativeEndian.PutUint64(b[i*ptrSize:], w)
	}
	return b
}

func TestFindGoFuncInModuleData(t *testing.T) {
	const (
		gofunc   = uint64(0x480000) // moduledata.gofunc value
		epclntab = uint64(0x4a0000) // gopclntab end (Address + len(Data))
	)

	tests := []struct {
		name   string
		module []byte
		want   uint64
		wantOK bool
	}{
		{
			name:   "gofunc immediately precedes epclntab",
			module: moduleDataOf(0x400000, 0x401000, gofunc, epclntab, 0x111111),
			want:   gofunc,
			wantOK: true,
		},
		{
			name:   "no epclntab present",
			module: moduleDataOf(0x400000, gofunc, 0x111111),
			wantOK: false,
		},
		{
			name:   "epclntab with no preceding word",
			module: moduleDataOf(epclntab),
			wantOK: false,
		},
		{
			name:   "empty moduledata",
			module: nil,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := findGoFuncInModuleData(tt.module, epclntab)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %#x)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Fatalf("gofunc = %#x, want %#x", got, tt.want)
			}
		})
	}
}

// syntheticPclntab116 builds a minimal Go 1.16 pclntab blob whose header is valid enough for parseGoPCLnTab to accept,
// but with the given numFuncs. All section offsets point to the end of the header so functab is empty.
func syntheticPclntab116(numFuncs uint64) []byte {
	const hdrSize = 64 // unsafe.Sizeof(pclntabHeader116{})
	b := make([]byte, hdrSize)
	binary.NativeEndian.PutUint32(b[0:], magicGo1_16)
	// pad (2 bytes) = 0, quantum (1 byte) = 0
	b[7] = ptrSize                                 // ptrSize field
	binary.NativeEndian.PutUint64(b[8:], numFuncs) // numFuncs
	// nfiles at offset 16 = 0 (unused by bound check)
	// all five uintptr offsets point to hdrSize so no "corrupt" error fires
	for _, off := range []int{24, 32, 40, 48, 56} {
		binary.NativeEndian.PutUint64(b[off:], hdrSize)
	}
	return b
}

// TestMalformedNumFuncsNoDoS guards the CPU-DoS bound: parseGoPCLnTab must reject an inflated numFuncs immediately
// (validated against the functab size) rather than spinning O(numFuncs) iterations (otel-ebpf-profiler#1602 class).
func TestMalformedNumFuncsNoDoS(t *testing.T) {
	const bigNumFuncs = uint64(1) << 62
	blob := syntheticPclntab116(bigNumFuncs)
	_, err := parseGoPCLnTab(blob)
	if err == nil || !strings.Contains(err.Error(), "exceeds functab capacity") {
		t.Fatalf("expected numFuncs bound error, got: %v", err)
	}
}

// TestMalformedNumFuncsMissingSentinel guards the sentinel-entry check: a functab with exactly
// 2*numFuncs*fieldSize bytes (pairs only, no trailing sentinel pc) must be rejected. The old bound
// `numFuncs > len(functab)/(2*fieldSize)` accepted such blobs; the correct check requires
// (2*numFuncs+1) fields.
func TestMalformedNumFuncsMissingSentinel(t *testing.T) {
	const hdrSize = 64 // unsafe.Sizeof(pclntabHeader116{})
	const fieldSize = ptrSize
	const numFuncs = uint64(4)

	// Build a functab with exactly 2*numFuncs fields (pairs only, sentinel missing).
	functabSize := int(2 * numFuncs * fieldSize)
	b := make([]byte, hdrSize+functabSize)
	binary.NativeEndian.PutUint32(b[0:], magicGo1_16)
	b[7] = ptrSize
	binary.NativeEndian.PutUint64(b[8:], numFuncs)
	// pclnOffset (offset 56) points to hdrSize, so functab starts right after the header.
	for _, off := range []int{24, 32, 40, 48, 56} {
		binary.NativeEndian.PutUint64(b[off:], hdrSize)
	}

	_, err := parseGoPCLnTab(b)
	if err == nil || !strings.Contains(err.Error(), "exceeds functab capacity") {
		t.Fatalf("expected sentinel bound error, got: %v", err)
	}
}
