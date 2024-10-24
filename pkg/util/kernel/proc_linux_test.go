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

	"github.com/stretchr/testify/require"
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

const mapsFileSample = `
08048000-08049000 r-xp 00000000 03:00 8312       /opt/test
08049000-0804a000 rw-p 00001000 03:00 8312       /opt/test
0804a000-0806b000 rw-p 00000000 00:00 0          [heap]
a7ed5000-a8008000 r-xp 00000000 03:00 4222       /lib/libc.so.6
a800b000-a800e000 rw-p 00000000 00:00 0
a800e000-a8022000 r-xp 00000000 03:00 14462      /lib/libpthread.so.0
a8024000-a8027000 rw-p 00000000 00:00 0
a8027000-a8043000 r-xp 00000000 03:00 8317       /lib/ld-linux.so.2
aff35000-aff4a000 rw-p 00000000 00:00 0          [stack]
ffffe000-fffff000 r-xp 00000000 00:00 0          [vdso]
01c00000-02000000 rw-p 00000000 00:0d 6123886    /anon_hugepage (deleted)
`

func TestReadProcessMemMaps(t *testing.T) {
	buffer := bytes.NewReader([]byte(mapsFileSample))
	entries, err := readProcessMemMapsFromBuffer(buffer)
	require.NoError(t, err)

	// Same as in the maps file, but sorted by start address
	expectedEntries := ProcMapEntries{
		{0x01c00000, 0x02000000, "rw-p", 0x00000000, 0x00, 0x0d, 6123886, "/anon_hugepage", true},
		{0x08048000, 0x08049000, "r-xp", 0x00000000, 0x03, 0x00, 8312, "/opt/test", false},
		{0x08049000, 0x0804a000, "rw-p", 0x00001000, 0x03, 0x00, 8312, "/opt/test", false},
		{0x0804a000, 0x0806b000, "rw-p", 0x00000000, 0x00, 0x00, 0, "[heap]", false},
		{0xa7ed5000, 0xa8008000, "r-xp", 0x00000000, 0x03, 0x00, 4222, "/lib/libc.so.6", false},
		{0xa800b000, 0xa800e000, "rw-p", 0x00000000, 0x00, 0x00, 0, "", false},
		{0xa800e000, 0xa8022000, "r-xp", 0x00000000, 0x03, 0x00, 14462, "/lib/libpthread.so.0", false},
		{0xa8024000, 0xa8027000, "rw-p", 0x00000000, 0x00, 0x00, 0, "", false},
		{0xa8027000, 0xa8043000, "r-xp", 0x00000000, 0x03, 0x00, 8317, "/lib/ld-linux.so.2", false},
		{0xaff35000, 0xaff4a000, "rw-p", 0x00000000, 0x00, 0x00, 0, "[stack]", false},
		{0xffffe000, 0xfffff000, "r-xp", 0x00000000, 0x00, 0x00, 0, "[vdso]", false},
	}

	// assert that the entries are the same
	require.Equal(t, expectedEntries, entries)
}
