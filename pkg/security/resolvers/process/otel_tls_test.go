// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package process

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

const mapsFileSample = `
08047000-08048000 rw-p 00000000 03:00 8312       /memfd:OTEL_CTX
08048000-08049000 r-xp 00000000 03:00 8312       /opt/test
08049000-0804a000 rw-p 00001000 03:00 8312       /opt/test
0804a000-0806b000 rw-p 00000000 00:00 0          [heap]
a7cb1000-a7cb2000 ---p 00000000 00:00 0
a7cb2000-a7eb2000 rw-p 00000000 00:00 0
a7eb2000-a7eb3000 ---p 00000000 00:00 0
a7eb3000-a7ed5000 rw-p 00000000 00:00 0
a7ed5000-a8008000 r-xp 00000000 03:00 4222       /lib/libc.so.6
a8008000-a800a000 r--p 00133000 03:00 4222       /lib/libc.so.6
a800a000-a800b000 rw-p 00135000 03:00 4222       /lib/libc.so.6
a800b000-a800e000 rw-p 00000000 00:00 0
a800e000-a8022000 r-xp 00000000 03:00 14462      /lib/libpthread.so.0
a8022000-a8023000 r--p 00013000 03:00 14462      /lib/libpthread.so.0
a8023000-a8024000 rw-p 00014000 03:00 14462      /lib/libpthread.so.0
a8024000-a8027000 rw-p 00000000 00:00 0
a8027000-a8043000 r-xp 00000000 03:00 8317       /lib/ld-linux.so.2
a8043000-a8044000 r--p 0001b000 03:00 8317       /lib/ld-linux.so.2
a8044000-a8045000 rw-p 0001c000 03:00 8317       /lib/ld-linux.so.2
aff35000-aff4a000 rw-p 00000000 00:00 0          [stack]
ffffe000-fffff000 r-xp 00000000 00:00 0          [vdso]
01c00000-02000000 rw-p 00000000 00:0d 6123886    /anon_hugepage (deleted)
`

func BenchmarkTLSCandidateObjects(b *testing.B) {
	pid := uint32(2)
	procfs := kernel.CreateFakeProcFS(b, []kernel.FakeProcFSEntry{{Pid: pid, Maps: mapsFileSample, Exe: "fake"}})
	kernel.WithFakeProcFS(b, procfs)

	load := func(b *testing.B, p *otelTargetProcess) {
		p.reset(pid)
		objects, err := p.tlsCandidateObjects()
		if err != nil {
			b.Fatal(err)
		}
		if len(objects) == 0 {
			b.Fatal("no candidate objects")
		}
	}

	// reused is what a resolution costs on the target a caller carries across
	// resolutions, which is how both callers use it. fresh is the
	// counterfactual of a target per resolution.
	b.Run("fresh", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			load(b, new(otelTargetProcess))
		}
	})

	b.Run("reused", func(b *testing.B) {
		p := new(otelTargetProcess)
		b.ReportAllocs()
		for b.Loop() {
			load(b, p)
		}
	})
}

// TestResetKeepsBackingArray pins the point of reusing a target: a second load
// must refill the object list in place. Building that list is otherwise free
// only because the array survives -- a maps read costs the same whether the
// list is built or not, so a reallocation here is pure added cost.
func TestResetKeepsBackingArray(t *testing.T) {
	pid := uint32(2)
	procfs := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: pid, Maps: mapsFileSample}})
	kernel.WithFakeProcFS(t, procfs)

	p := new(otelTargetProcess)
	p.reset(pid)
	_, err := p.tlsCandidateObjects()
	require.NoError(t, err)
	require.NotEmpty(t, p.mapsObjects)
	first := &p.mapsObjects[0]

	p.reset(pid)
	_, err = p.tlsCandidateObjects()
	require.NoError(t, err)
	require.NotEmpty(t, p.mapsObjects)
	require.Same(t, first, &p.mapsObjects[0], "reset must refill the object list in place")
}

func TestTLSCandidateObjects(t *testing.T) {
	pid := uint32(2)
	procfs := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: pid, Maps: mapsFileSample}})
	kernel.WithFakeProcFS(t, procfs)

	p := new(otelTargetProcess)
	p.reset(pid)
	objects, err := p.tlsCandidateObjects()
	require.NoError(t, err)

	paths := make([]string, 0, len(objects))
	for _, obj := range objects {
		paths = append(paths, obj.path)
	}
	// First-mapped order, one entry per distinct object, " (deleted)" stripped.
	// Only executable file-backed mappings qualify, so the read-write
	// /memfd:OTEL_CTX and /anon_hugepage mappings and the anonymous [vdso] are
	// all absent.
	require.Equal(t, []string{
		"/opt/test",
		"/lib/libc.so.6",
		"/lib/libpthread.so.0",
		"/lib/ld-linux.so.2",
	}, paths)

	byPath := func(path string) otelMappedObject {
		for _, obj := range objects {
			if obj.path == path {
				return obj
			}
		}
		t.Fatalf("no object for %s", path)
		return otelMappedObject{}
	}

	// The anchor is the object's first executable mapping, not its first.
	require.Equal(t, uint64(0xa7ed5000), byPath("/lib/libc.so.6").exec.StartAddr)
	require.Equal(t, "r-xp", byPath("/lib/libc.so.6").exec.Permissions)

	// The context mapping is found even though it is not a candidate itself:
	// the publisher maps it read-write, and the scan has to reach it wherever
	// the address space puts it.
	require.Equal(t, uint64(0x08047000), p.procCtxAddr)
}

// TestOTelTargetProcessReset pins the reset of every memoized field, since a
// reused target that keeps one silently reports the previous process's state.
func TestOTelTargetProcessReset(t *testing.T) {
	pid := uint32(2)
	procfs := kernel.CreateFakeProcFS(t, []kernel.FakeProcFSEntry{{Pid: pid, Maps: mapsFileSample}})
	kernel.WithFakeProcFS(t, procfs)

	p := new(otelTargetProcess)
	p.reset(pid)
	_, err := p.tlsCandidateObjects()
	require.NoError(t, err)
	_, _ = p.exe()
	require.NotEmpty(t, p.mapsObjects)

	p.reset(3)
	require.Equal(t, uint32(3), p.pid)
	require.Equal(t, "3", p.pidStr)
	require.Empty(t, p.mapsObjects)
	require.False(t, p.mapsDone)
	require.NoError(t, p.mapsErr)
	require.Empty(t, p.exePath)
	require.False(t, p.exeDone)
	require.NoError(t, p.exeErr)
	require.Zero(t, p.procCtxAddr)
}
