// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package procmaps

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/sys/windows"
)

func TestProtectToMode(t *testing.T) {
	cases := map[uint32]string{
		windows.PAGE_READONLY:          "r----",
		windows.PAGE_READWRITE:         "rw---",
		windows.PAGE_WRITECOPY:         "rw---",
		windows.PAGE_EXECUTE:           "--x--",
		windows.PAGE_EXECUTE_READ:      "r-x--",
		windows.PAGE_EXECUTE_READWRITE: "rwx--",
		windows.PAGE_EXECUTE_WRITECOPY: "rwx--",
		0:                              "-----",
	}
	for protect, want := range cases {
		assert.Equal(t, want, protectToMode(protect), "protect %#x", protect)
	}
}

// TestProtectToModeMasksModifierBits ensures PAGE_GUARD/PAGE_NOCACHE/
// PAGE_WRITECOMBINE (which can be OR'd onto any base protection constant)
// are stripped before classification, so a guard page still reports its
// underlying protection rather than falling through to "-----".
func TestProtectToModeMasksModifierBits(t *testing.T) {
	cases := []struct {
		name     string
		protect  uint32
		wantMode string
	}{
		{"readonly+guard", windows.PAGE_READONLY | windows.PAGE_GUARD, "r----"},
		{"readwrite+nocache", windows.PAGE_READWRITE | windows.PAGE_NOCACHE, "rw---"},
		{"executeread+writecombine", windows.PAGE_EXECUTE_READ | windows.PAGE_WRITECOMBINE, "r-x--"},
		{"executereadwrite+all-modifiers", windows.PAGE_EXECUTE_READWRITE | windows.PAGE_GUARD | windows.PAGE_NOCACHE | windows.PAGE_WRITECOMBINE, "rwx--"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantMode, protectToMode(tc.protect))
		})
	}
}

func TestRegionName(t *testing.T) {
	cases := map[uint32]string{
		memImage:  "[image]",
		memMapped: "[ mapped ]",
		0x20000:   "[ anon ]", // MEM_PRIVATE has no dedicated MEMORY_BASIC_INFORMATION.Type value
		0:         "[ anon ]",
	}
	for typ, want := range cases {
		assert.Equal(t, want, regionName(typ), "type %#x", typ)
	}
}
