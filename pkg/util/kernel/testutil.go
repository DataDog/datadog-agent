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
func WithFakeProcFS(t *testing.T, procRoot string) {
	resetProcFSRoot()
	t.Setenv("HOST_PROC", procRoot)
	t.Cleanup(func() {
		resetProcFSRoot()
	})
}

// CreateFakeProcFS creates a fake /proc filesystem with the given entries
func CreateFakeProcFS(t *testing.T, entries []FakeProcFSEntry) string {
	procRoot := t.TempDir()

	for _, entry := range entries {
		baseDir := filepath.Join(procRoot, strconv.Itoa(int(entry.Pid)))

		createFile(t, filepath.Join(baseDir, "cmdline"), entry.Cmdline)
		createFile(t, filepath.Join(baseDir, "comm"), entry.Command)
		createFile(t, filepath.Join(baseDir, "maps"), entry.Maps)
		createSymlink(t, entry.Exe, filepath.Join(baseDir, "exe"))
		createFile(t, filepath.Join(baseDir, "environ"), entry.getEnvironContents())
	}

	return procRoot
}

// FakeProcFSEntry represents a fake /proc filesystem entry for testing purposes.
type FakeProcFSEntry struct {
	Pid     uint32
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

func createFile(t *testing.T, path, data string) {
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.WriteFile(path, []byte(data), 0775))
}

func createSymlink(t *testing.T, target, link string) {
	if target == "" {
		return
	}

	dir := filepath.Dir(link)
	require.NoError(t, os.MkdirAll(dir, 0775))
	require.NoError(t, os.Symlink(target, link))
}
