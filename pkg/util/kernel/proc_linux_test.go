// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
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
		{
			name:     "LastVarWithNoTrailingNull",
			contents: "PATH=/usr/bin\x00MY_VAR=myvar",
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

// A memfd is found wherever it sits in the descriptor table, including past the
// first chunk of the directory, which is the case the quick path misses.
func TestFindMemFdFilePath(t *testing.T) {
	const memFdName = "dd_test_memfd_search"

	for i := 0; i < fdChunkSize+16; i++ {
		f, err := os.Open(os.DevNull)
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })
	}

	fd, err := unix.MemfdCreate(memFdName, 0)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(fd) })
	// Otherwise the search would find it without ever paging the directory.
	require.Greater(t, fd, fdChunkSize)

	path, found := findMemFdFilePath(os.Getpid(), "/proc", memFdName)
	require.True(t, found)
	require.Equal(
		t,
		filepath.Join("/proc", strconv.Itoa(os.Getpid()), "fd", strconv.Itoa(fd)),
		path,
	)

	_, found = findMemFdFilePath(os.Getpid(), "/proc", "dd_test_memfd_absent")
	require.False(t, found)
}
