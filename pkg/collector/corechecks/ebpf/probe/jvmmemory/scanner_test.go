// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package jvmmemory

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
)

const jvmMaps = `55d0c0a00000-55d0c0a01000 r--p 00000000 00:2f 1234 /opt/java/openjdk/bin/java
7f3a10000000-7f3a10021000 rw-p 00000000 00:00 0
7f3a11000000-7f3a11200000 r--p 00000000 00:2f 5678 /opt/java/openjdk/lib/server/libjvm.so
7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /opt/java/openjdk/lib/server/libjvm.so
7ffd5e1f0000-7ffd5e211000 rw-p 00000000 00:00 0                          [stack]
`

const shellMaps = `55d0c0a00000-55d0c0a01000 r--p 00000000 08:01 1234 /usr/bin/bash
7f3a11000000-7f3a11200000 r--p 00000000 08:01 5678 /usr/lib/x86_64-linux-gnu/libc.so.6
`

func TestParseLibJVMMapping(t *testing.T) {
	for _, tc := range []struct {
		line string
		path string
	}{
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /opt/java/openjdk/lib/server/libjvm.so", path: "/opt/java/openjdk/lib/server/libjvm.so"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678     /usr/lib/jvm/java 21/lib/server/libjvm.so", path: "/usr/lib/jvm/java 21/lib/server/libjvm.so"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /usr/lib/jvm/lib/server/libjvm.so (deleted)"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:00 0 /memfd:libjvm.so"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /opt/libjvm.so.1"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /opt/not-libjvm.so"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /opt/libjvm.so/libc.so"},
		{line: "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678"},
		{line: "libjvm.so"},
		{line: ""},
	} {
		path, ok := parseLibJVMMapping(tc.line)
		assert.Equal(t, tc.path != "", ok, tc.line)
		assert.Equal(t, tc.path, path, tc.line)
	}
}

// fakeProcFS is a procfs with process directories only.
type fakeProcFS struct {
	t    *testing.T
	root string
}

func newFakeProcFS(t *testing.T) *fakeProcFS {
	return &fakeProcFS{t: t, root: t.TempDir()}
}

const defaultStartTime = 1000

func (p *fakeProcFS) setProcess(pid uint32, comm, maps string) {
	p.setProcessStartedAt(pid, comm, maps, defaultStartTime)
}

func (p *fakeProcFS) setProcessStartedAt(pid uint32, comm, maps string, startTime uint64) {
	p.writeFile(pid, "comm", comm+"\n")
	p.writeFile(pid, "maps", maps)
	p.writeFile(pid, "stat", statLine(pid, comm, startTime))
}

// writeFile writes the file name of the directory of pid.
func (p *fakeProcFS) writeFile(pid uint32, name, content string) {
	dir := procPidPath(p.root, pid)
	require.NoError(p.t, os.MkdirAll(dir, 0o755))
	require.NoError(p.t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
}

// statLine returns a /proc/<pid>/stat line with the given comm and start time.
func statLine(pid uint32, comm string, startTime uint64) string {
	return fmt.Sprintf("%d (%s) S 1 %d %d 0 -1 4194560 6170 0 0 0 51 12 0 0 20 0 30 0 %d 3891200000 41765 18446744073709551615 1 1 0 0 0 0 0 4096 16796879 0 0 0 17 3 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
		pid, comm, pid, pid, startTime)
}

func (p *fakeProcFS) removeProcess(pid uint32) {
	require.NoError(p.t, os.RemoveAll(procPidPath(p.root, pid)))
}

type registration struct {
	path string
	pid  uint32
}

// fakeRegistry records the registrations, and returns the next error of errs.
type fakeRegistry struct {
	registered   []registration
	unregistered []uint32
	errs         []error
}

func (r *fakeRegistry) Register(namespacedPath string, pid uint32, activationCB, deactivationCB, alreadyRegistered utils.Callback) error {
	if activationCB == nil || deactivationCB == nil || alreadyRegistered == nil {
		return errors.New("missing callback")
	}
	r.registered = append(r.registered, registration{path: namespacedPath, pid: pid})
	if len(r.errs) == 0 {
		return nil
	}
	err := r.errs[0]
	r.errs = r.errs[1:]
	return err
}

func (r *fakeRegistry) Unregister(pid uint32) error {
	r.unregistered = append(r.unregistered, pid)
	return nil
}

func newTestScanner(procfs *fakeProcFS, registry *fakeRegistry) *processScanner {
	return newProcessScanner(procfs.root, registry, utils.IgnoreCB, utils.IgnoreCB)
}

func mustScan(t *testing.T, s *processScanner) scanResult {
	t.Helper()
	res, err := s.scan()
	require.NoError(t, err)
	return res
}

func TestScannerRegistersJVMs(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{}
	s := newTestScanner(procfs, registry)

	procfs.setProcess(10, "java", jvmMaps)
	procfs.setProcess(11, "bash", shellMaps)
	res := mustScan(t, s)
	assert.Equal(t, map[uint32]struct{}{10: {}, 11: {}}, res.alive)
	assert.Equal(t, 1, res.jvms)
	assert.Equal(t, []registration{{path: "/opt/java/openjdk/lib/server/libjvm.so", pid: 10}}, registry.registered)

	// a registered JVM is not inspected again
	res = mustScan(t, s)
	assert.Equal(t, 1, res.jvms)
	assert.Len(t, registry.registered, 1)

	// exited processes are forgotten, the JVMs unregistered
	procfs.removeProcess(10)
	procfs.removeProcess(11)
	res = mustScan(t, s)
	assert.Empty(t, res.alive)
	assert.Zero(t, res.jvms)
	assert.Equal(t, []uint32{10}, registry.unregistered)
	assert.Empty(t, s.pids)
}

func TestScannerInspectsProcessesTwice(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{}
	s := newTestScanner(procfs, registry)

	// libjvm.so is loaded between the first and the second scan
	procfs.setProcess(20, "java", shellMaps)
	mustScan(t, s)
	assert.Empty(t, registry.registered)
	procfs.setProcess(20, "java", jvmMaps)
	mustScan(t, s)
	assert.Equal(t, []registration{{path: "/opt/java/openjdk/lib/server/libjvm.so", pid: 20}}, registry.registered)

	// a process that maps no libjvm.so at two scans is not a JVM
	procfs.setProcess(21, "bash", shellMaps)
	mustScan(t, s)
	mustScan(t, s)
	procfs.setProcess(21, "bash", jvmMaps)
	mustScan(t, s)
	assert.Len(t, registry.registered, 1)
	assert.Equal(t, maxInspections, s.pids[21].inspections)

	// until it exec's another program
	procfs.setProcess(21, "java", jvmMaps)
	mustScan(t, s)
	assert.Equal(t, []registration{
		{path: "/opt/java/openjdk/lib/server/libjvm.so", pid: 20},
		{path: "/opt/java/openjdk/lib/server/libjvm.so", pid: 21},
	}, registry.registered)
	assert.True(t, s.pids[21].registered)
}

func TestScannerRegistrationErrors(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{errs: []error{utils.ErrPathIsAlreadyRegistered, errors.New("no HotSpot probe")}}
	s := newTestScanner(procfs, registry)

	procfs.setProcess(30, "java", jvmMaps)
	procfs.setProcess(31, "java", jvmMaps)
	res := mustScan(t, s)
	require.Len(t, registry.registered, 2)
	// the registry tracks the PIDs of an already registered file
	assert.Equal(t, 1, res.jvms)

	// a JVM which could not be registered is not retried
	mustScan(t, s)
	assert.Len(t, registry.registered, 2)

	procfs.removeProcess(30)
	procfs.removeProcess(31)
	mustScan(t, s)
	assert.Len(t, registry.unregistered, 1)
	assert.Contains(t, []uint32{30, 31}, registry.unregistered[0])
}

func TestScannerRetriesTransientRegistrationErrors(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{errs: []error{
		retryLater(errors.New("too many files")),
		nil,
		// e.g. no probe site could be attached: not retried
		fmt.Errorf("attach: %w", utils.ErrEnvironment),
	}}
	s := newTestScanner(procfs, registry)

	procfs.setProcess(35, "java", jvmMaps)
	res := mustScan(t, s)
	assert.Zero(t, res.jvms)
	assert.False(t, s.pids[35].registered)

	// retried at every scan until it succeeds
	res = mustScan(t, s)
	assert.Equal(t, 1, res.jvms)
	assert.Len(t, registry.registered, 2)
	assert.True(t, s.pids[35].registered)

	// other environment errors are not retried
	procfs.setProcess(34, "java", jvmMaps)
	mustScan(t, s)
	mustScan(t, s)
	assert.Len(t, registry.registered, 3)
	assert.False(t, s.pids[34].registered)
}

func TestScannerDetectsPIDReuse(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{}
	s := newTestScanner(procfs, registry)

	procfs.setProcessStartedAt(36, "java", jvmMaps, 100)
	res := mustScan(t, s)
	assert.Empty(t, res.reused)
	assert.Equal(t, 1, res.jvms)

	// same process
	res = mustScan(t, s)
	assert.Empty(t, res.reused)
	assert.Empty(t, registry.unregistered)

	// the JVM exited and another JVM reused its PID between two scans: the
	// former is unregistered and reported, the latter registered
	procfs.setProcessStartedAt(36, "java", jvmMaps, 200)
	res = mustScan(t, s)
	assert.Equal(t, map[uint32]struct{}{36: {}}, res.reused)
	assert.Equal(t, []uint32{36}, registry.unregistered)
	assert.Len(t, registry.registered, 2)
	assert.Equal(t, 1, res.jvms)
	assert.Equal(t, uint64(200), s.pids[36].startTime)
}

func TestReadStartTime(t *testing.T) {
	procfs := newFakeProcFS(t)
	// the comm may contain spaces and parentheses
	procfs.setProcessStartedAt(37, "a) (b c", jvmMaps, 123456)
	startTime, err := readStartTime(procfs.root, 37)
	require.NoError(t, err)
	assert.Equal(t, uint64(123456), startTime)

	procfs.writeFile(37, "stat", "37 (java) S 1")
	_, err = readStartTime(procfs.root, 37)
	assert.Error(t, err)

	_, err = readStartTime(procfs.root, 38)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestScannerSkipsVanishedProcesses(t *testing.T) {
	procfs := newFakeProcFS(t)
	registry := &fakeRegistry{}
	s := newTestScanner(procfs, registry)

	// a process directory without maps file, e.g. a process exiting
	require.NoError(t, os.MkdirAll(procPidPath(procfs.root, 40), 0o755))
	// not a PID
	require.NoError(t, os.MkdirAll(filepath.Join(procfs.root, "self"), 0o755))
	res := mustScan(t, s)
	assert.Equal(t, map[uint32]struct{}{40: {}}, res.alive)
	assert.Empty(t, registry.registered)
}

func TestFindLibJVM(t *testing.T) {
	procfs := newFakeProcFS(t)
	// a long maps file, as the ones of JVMs
	var maps strings.Builder
	for i := 0; i < 5000; i++ {
		maps.WriteString("7f3a10000000-7f3a10021000 rw-p 00000000 00:00 0 \n")
	}
	maps.WriteString(jvmMaps)
	procfs.setProcess(50, "java", maps.String())
	s := newTestScanner(procfs, &fakeRegistry{})

	path, err := s.findLibJVM(50)
	require.NoError(t, err)
	assert.Equal(t, "/opt/java/openjdk/lib/server/libjvm.so", path)

	procfs.setProcess(51, "bash", shellMaps)
	path, err = s.findLibJVM(51)
	require.NoError(t, err)
	assert.Empty(t, path)

	_, err = s.findLibJVM(52)
	assert.ErrorIs(t, err, os.ErrNotExist)

	// a line longer than the initial line buffer
	longPath := "/" + strings.Repeat("d", 2*mapsBufSize) + "/lib/server/libjvm.so"
	procfs.setProcess(53, "java", shellMaps+"7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 "+longPath+"\n")
	path, err = s.findLibJVM(53)
	require.NoError(t, err)
	assert.Equal(t, longPath, path)

	path, err = s.findLibJVM(50)
	require.NoError(t, err)
	assert.Equal(t, "/opt/java/openjdk/lib/server/libjvm.so", path)

	// a line longer than the cap
	procfs.setProcess(54, "java", "7f3a11200000-7f3a12000000 r-xp 00200000 00:2f 5678 /"+strings.Repeat("d", maxMapsLineLen)+"\n")
	_, err = s.findLibJVM(54)
	assert.ErrorIs(t, err, bufio.ErrTooLong)
}
