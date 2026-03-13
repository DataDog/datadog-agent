// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && test

package kernel

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// WithFakeProcFS sets the procfs root to the given path for the duration of the
// test. procRoot should be a path to a directory that contains the /proc
// filesystem. CreateFakeProcFS can be used to create a fake /proc filesystem.
// in a structured manner
func WithFakeProcFS(tb testing.TB, procRoot string) {
	resetProcFSRoot()
	tb.Setenv("HOST_PROC", procRoot)
	tb.Cleanup(func() {
		resetProcFSRoot()
	})
}

// FakeProcFSOption is a function that modifies a fake proc filesystem
type FakeProcFSOption func(tb testing.TB, procRoot string)

// WithRealUptime links the host uptime to the fake procfs uptime
func WithRealUptime() func(testing.TB, string) {
	return func(tb testing.TB, procRoot string) {
		createSymlink(tb, "/proc/uptime", filepath.Join(procRoot, "uptime"))
	}
}

// WithRealStat links the host stat to the fake procfs stat
func WithRealStat() func(testing.TB, string) {
	return func(tb testing.TB, procRoot string) {
		createSymlink(tb, "/proc/stat", filepath.Join(procRoot, "stat"))
	}
}

// CreateFakeProcFS creates a fake /proc filesystem with the given entries
func CreateFakeProcFS(t *testing.T, entries []FakeProcFSEntry, options ...FakeProcFSOption) string {
	procRoot := t.TempDir()

	for _, entry := range entries {
		baseDir := filepath.Join(procRoot, strconv.Itoa(int(entry.Pid)))
		mainTaskDir := filepath.Join(baseDir, "task", strconv.Itoa(int(entry.Pid)))

		createFile(t, filepath.Join(baseDir, "cmdline"), entry.Cmdline)
		createFile(t, filepath.Join(baseDir, "comm"), entry.Command)
		createFile(t, filepath.Join(baseDir, "maps"), entry.Maps)
		createSymlink(t, entry.Exe, filepath.Join(baseDir, "exe"))
		createFile(t, filepath.Join(baseDir, "environ"), entry.getEnvironContents())
		createFile(t, filepath.Join(mainTaskDir, "status"), entry.getMainTaskStatusContent())
	}

	for _, option := range options {
		option(t, procRoot)
	}

	return procRoot
}

// FakeProcFSEntry represents a fake /proc filesystem entry for testing purposes.
type FakeProcFSEntry struct {
	Pid     uint32
	NsPid   uint32
	Cmdline string
	Command string
	Exe     string
	Maps    string
	Env     map[string]string
}

// getEnvironContents returns the formatted contents of the /proc/<pid>/environ file for the entry.
func (f *FakeProcFSEntry) getEnvironContents() string {
	if len(f.Env) == 0 {
		return ""
	}

	formattedEnvVars := make([]string, 0, len(f.Env))
	for k, v := range f.Env {
		formattedEnvVars = append(formattedEnvVars, fmt.Sprintf("%s=%s", k, v))
	}

	return strings.Join(formattedEnvVars, "\x00") + "\x00"
}

// getMainTaskStatusContent returns the formatted contents of the /proc/<pid>/task/<pid>/status file for the entry.
func (f *FakeProcFSEntry) getMainTaskStatusContent() string {
	content := fmt.Sprintf("Pid: %d\n", f.Pid)

	if f.NsPid != 0 {
		content += fmt.Sprintf("NSpid: %d\n", f.NsPid)
	}

	return content
}

func createFile(tb testing.TB, path, data string) {
	dir := filepath.Dir(path)
	require.NoError(tb, os.MkdirAll(dir, 0775))
	require.NoError(tb, os.WriteFile(path, []byte(data), 0775))
}

func createSymlink(tb testing.TB, target, link string) {
	if target == "" {
		return
	}

	dir := filepath.Dir(link)
	require.NoError(tb, os.MkdirAll(dir, 0775))
	require.NoError(tb, os.Symlink(target, link))
}
