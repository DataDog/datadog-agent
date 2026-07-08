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
