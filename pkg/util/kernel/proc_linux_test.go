// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"testing"
)

func oldWithAllProcs(procRoot string, fn func(int) error) error {
	files, err := os.ReadDir(procRoot)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.IsDir() || f.Name() == "." || f.Name() == ".." {
			continue
		}

		var pid int
		if pid, err = strconv.Atoi(f.Name()); err != nil {
			continue
		}

		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkOldWithAllProcs(b *testing.B) {

	var pids []int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids := []int{}
		oldWithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkWithAllProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids = []int{}
		WithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkAllPidsProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids, _ = AllPidsProcs("/proc")
	}
	runtime.KeepAlive(pids)
}

func TestGetEnvVariableFromBuffer(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		envVar   string
		expected string
	}{
		{
			name:     "NonExistent",
			contents: "PATH=/usr/bin\x00HOME=/home/user\x00",
			envVar:   "NONEXISTENT",
			expected: "",
		},
		{
			name:     "Exists",
			contents: "PATH=/usr/bin\x00MY_VAR=myvar\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "myvar",
		},
		{
			name:     "Empty",
			contents: "PATH=/usr/bin\x00MY_VAR=\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "",
		},
		{
			name:     "PrefixVarNotSelected",
			contents: "PATH=/usr/bin\x00MY_VAR_BUT_NOT_THIS=nope\x00MY_VAR=myvar\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "myvar",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getEnvVariableFromBuffer(bytes.NewBufferString(tc.contents), tc.envVar)
			if actual != tc.expected {
				t.Fatalf("Expected %s, got %s", tc.expected, actual)
			}
		})
	}
}
