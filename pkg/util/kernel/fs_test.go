// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"runtime"
	"testing"
)

func BenchmarkHostProc(b *testing.B) {
	_ = ProcFSRoot()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res := hostProcInternal("/proc", "a", "b", "c", "d")
		runtime.KeepAlive(res)
	}
}

func TestHostProc(t *testing.T) {
	type testCase struct {
		name       string
		procFsRoot string
		args       []string
		expected   string
	}

	entries := []testCase{
		{"empty", "/proc", nil, "/proc"},
		{"single", "/proc", []string{"a"}, "/proc/a"},
		{"multiple", "/host/proc", []string{"a", "b", "c", "d"}, "/host/proc/a/b/c/d"},

		{"slash", "/proc", []string{"/a/b"}, "/proc/a/b"},
		{"slash-empty", "/host/proc", []string{"/"}, "/host/proc"},

		{"empty-host-proc", "", []string{"a", "b"}, "a/b"},
		{"everything-empty", "", nil, ""},
	}

	for _, entry := range entries {
		t.Run(entry.name, func(t *testing.T) {
			got := hostProcInternal(entry.procFsRoot, entry.args...)
			if entry.expected != got {
				t.Errorf("expected: `%v`, got: `%v`", entry.expected, got)
			}
		})
	}
}
