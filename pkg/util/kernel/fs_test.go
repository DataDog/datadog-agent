// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func BenchmarkHostProc(b *testing.B) {
	_ = ProcFSRoot()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res := HostProc("a", "b", "c", "d")
		runtime.KeepAlive(res)
	}
}

func TestHostProc(t *testing.T) {
	entries := [][]string{
		nil,
		{"a", "b", "c", "d"},
		{"self", "mountinfo"},
		{"10", "fd", "3"},
		{"a"},
	}

	legacyHostProc := func(combineWith ...string) string {
		return filepath.Join(ProcFSRoot(), filepath.Join(combineWith...))
	}

	for _, entry := range entries {
		t.Run(strings.Join(entry, "/"), func(t *testing.T) {
			v1 := HostProc(entry...)
			v2 := legacyHostProc(entry...)
			if v1 != v2 {
				t.Errorf("v1: %v, v1: %v", v1, v2)
			}
		})
	}
}
