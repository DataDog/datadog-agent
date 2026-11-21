// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package apmattachimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeProc struct {
	t    *testing.T
	root string
}

func newFakeProc(t *testing.T) *fakeProc {
	return &fakeProc{t: t, root: t.TempDir()}
}

type fakeProcess struct {
	exe       string
	cmdline   []string
	comm      string
	startTime uint64
	noStat    bool
}

func statLine(pid int, comm string, startTime uint64) string {
	// Fields 3..21 are filler; field 22 is the start time.
	return fmt.Sprintf("%d (%s) S%s %d 0 0\n", pid, comm, strings.Repeat(" 0", 21-4+1), startTime)
}

func (f *fakeProc) add(pid int, p fakeProcess) {
	dir := filepath.Join(f.root, strconv.Itoa(pid))
	require.NoError(f.t, os.MkdirAll(dir, 0o755))
	if p.exe != "" {
		require.NoError(f.t, os.Symlink(p.exe, filepath.Join(dir, "exe")))
	}
	if len(p.cmdline) > 0 {
		var data []byte
		for _, arg := range p.cmdline {
			data = append(append(data, arg...), 0)
		}
		require.NoError(f.t, os.WriteFile(filepath.Join(dir, "cmdline"), data, 0o644))
	}
	comm := p.comm
	if comm == "" {
		comm = "proc"
	}
	require.NoError(f.t, os.WriteFile(filepath.Join(dir, "comm"), []byte(comm+"\n"), 0o644))
	if !p.noStat {
		require.NoError(f.t, os.WriteFile(filepath.Join(dir, "stat"), []byte(statLine(pid, comm, p.startTime)), 0o644))
	}
}

func (f *fakeProc) remove(pid int) {
	require.NoError(f.t, os.RemoveAll(filepath.Join(f.root, strconv.Itoa(pid))))
}

func TestListJavaProcesses(t *testing.T) {
	proc := newFakeProc(t)
	proc.add(100, fakeProcess{exe: "/usr/lib/jvm/java-21/bin/java", comm: "java", startTime: 1000})
	proc.add(101, fakeProcess{exe: "/usr/bin/python3", comm: "python3", startTime: 1001})
	proc.add(102, fakeProcess{cmdline: []string{"/opt/jdk/bin/java", "-jar", "app.jar"}, startTime: 1002})
	proc.add(103, fakeProcess{comm: "java", startTime: 1003})
	proc.add(104, fakeProcess{exe: "/usr/bin/java", noStat: true})
	proc.add(105, fakeProcess{exe: "/usr/bin/javac", comm: "javac", startTime: 1005})
	proc.add(106, fakeProcess{exe: "/usr/lib/jvm/java-17/bin/java (deleted)", comm: "java", startTime: 1006})
	require.NoError(t, os.MkdirAll(filepath.Join(proc.root, "self"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(proc.root, "uptime"), []byte("1 1\n"), 0o644))

	got, err := listJavaProcesses(proc.root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []candidate{
		{key: procKey{pid: 100, startTime: 1000}, exe: "/usr/lib/jvm/java-21/bin/java"},
		{key: procKey{pid: 102, startTime: 1002}, exe: "/opt/jdk/bin/java"},
		{key: procKey{pid: 103, startTime: 1003}, exe: "java"},
		{key: procKey{pid: 106, startTime: 1006}, exe: "/usr/lib/jvm/java-17/bin/java"},
	}, got)
}

func TestListJavaProcessesMissingRoot(t *testing.T) {
	_, err := listJavaProcesses(filepath.Join(t.TempDir(), "nope"))
	assert.Error(t, err)
}

func TestParseStatStartTime(t *testing.T) {
	st, err := parseStatStartTime([]byte(statLine(42, "weird ) name (x", 987654)))
	require.NoError(t, err)
	assert.Equal(t, uint64(987654), st)

	_, err = parseStatStartTime([]byte("42 (java) S 1 2 3"))
	assert.Error(t, err)
	_, err = parseStatStartTime([]byte("garbage"))
	assert.Error(t, err)
}
