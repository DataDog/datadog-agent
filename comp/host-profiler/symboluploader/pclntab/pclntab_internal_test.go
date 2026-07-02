// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package pclntab

import (
	"encoding/binary"
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

// TestFindGoFuncInModuleData covers the Go 1.26+ recovery of the real goFunc
// address from moduledata. goFunc is identified by value: it is the smallest
// pointer in [minGoFunc, maxGoFunc[ = [functab end, gopclntab end[. This must
// return the exact stored value regardless of any alignment padding the linker
// inserted between functab and go:func.* (which would make the naive
// Address+funcTabEndOffset() computation land in the padding instead), and it
// must not depend on where the gofunc field sits in moduledata.
func TestFindGoFuncInModuleData(t *testing.T) {
	const (
		minGoFunc = uint64(0x480000) // functab end
		maxGoFunc = uint64(0x4a0000) // gopclntab end (Address + len(Data))
	)

	tests := []struct {
		name   string
		module []byte
		want   uint64
		wantOK bool
	}{
		{
			name:   "no padding: gofunc == functab end",
			module: moduleDataOf(0x400000, 0x401000, minGoFunc, 0x4a0000),
			want:   minGoFunc,
			wantOK: true,
		},
		{
			name: "8-byte alignment padding: gofunc is 8 past functab end",
			// The naive computation would return minGoFunc (in the padding);
			// the stored field holds the real value minGoFunc+8.
			module: moduleDataOf(0x400000, minGoFunc + 8, 0x4a0000),
			want:   minGoFunc + 8,
			wantOK: true,
		},
		{
			name: "field position does not matter (gofunc near the end)",
			module: moduleDataOf(0x400000, 0x401000, 0x402000, 0x403000, minGoFunc + 16),
			want:   minGoFunc + 16,
			wantOK: true,
		},
		{
			name: "smallest in-window pointer wins over a farther in-window value",
			// A larger unrelated pointer also falls in the window; gofunc is the
			// smaller one (nothing points into the [functab end, gofunc[ padding).
			module: moduleDataOf(minGoFunc + 200, minGoFunc + 8, 0x4a0000),
			want:   minGoFunc + 8,
			wantOK: true,
		},
		{
			name:   "pointer just below the window is ignored",
			module: moduleDataOf(minGoFunc - ptrSize, 0x4a0000),
			wantOK: false,
		},
		{
			name:   "pointer at/after the window end is ignored",
			module: moduleDataOf(maxGoFunc, maxGoFunc + 0x1000),
			wantOK: false,
		},
		{
			name:   "no pointer in window",
			module: moduleDataOf(0x400000, 0x401000, 0x4a0000),
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
			got, ok := findGoFuncInModuleData(tt.module, minGoFunc, maxGoFunc)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v (got %#x)", ok, tt.wantOK, got)
			}
			if ok && got != tt.want {
				t.Fatalf("gofunc = %#x, want %#x", got, tt.want)
			}
		})
	}
}

// TestFindGoFuncInModuleDataTightWindow locks the alignment boundary: with an
// Addralign=16 window, the functab end is ptrSize-aligned, so the largest possible
// padding is Addralign-ptrSize. goFunc then sits at hi-ptrSize and must still be
// matched (not skipped by the w >= hi check), while a decoy exactly at hi is excluded.
func TestFindGoFuncInModuleDataTightWindow(t *testing.T) {
	const lo = uint64(0x480000)
	const hi = lo + 16 // Addralign = 16

	got, ok := findGoFuncInModuleData(moduleDataOf(0x400000, lo+8, hi), lo, hi)
	if !ok || got != lo+8 {
		t.Fatalf("got (%#x, %v), want (%#x, true)", got, ok, lo+8)
	}
}
