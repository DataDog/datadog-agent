// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package units

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNames(t *testing.T) {
	service := Unit{Name: "datadog-agent-trace.service"}
	socket := Unit{Name: "datadog-agent-rshell-privileged.socket"}
	assert.Equal(t, "datadog-agent-trace-exp.service", service.ExpName())
	assert.Equal(t, []string{"datadog-agent-trace.service", "datadog-agent-rshell-privileged.socket", "datadog-agent-trace-exp.service", "datadog-agent-rshell-privileged-exp.socket"},
		StableAndExpNames(service, socket))
}

func TestWriteFileReplacesAtomically(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "processes.d")
	require.NoError(t, WriteFile(dir, "a.yaml", []byte("old")))
	require.NoError(t, WriteFile(dir, "a.yaml", []byte("new")))

	content, err := os.ReadFile(filepath.Join(dir, "a.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "new", string(content))
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "a.yaml"))
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "the temporary file must not be left behind")
}

func TestRemoveFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, WriteFile(dir, "a.yaml", nil))
	require.NoError(t, WriteFile(dir, "b.yaml", nil))

	require.NoError(t, RemoveFiles(dir, "a.yaml", "missing.yaml"))
	assert.NoFileExists(t, filepath.Join(dir, "a.yaml"))
	require.NoError(t, RemoveDirIfEmpty(dir))
	assert.DirExists(t, dir)

	require.NoError(t, RemoveFiles(dir, "b.yaml"))
	require.NoError(t, RemoveDirIfEmpty(dir))
	assert.NoDirExists(t, dir)
	assert.NoError(t, RemoveFiles(dir, "b.yaml"), "a missing directory is not an error")
}

func TestRemoveFilesDoesNotFollowSymlinkedDir(t *testing.T) {
	target := t.TempDir()
	require.NoError(t, WriteFile(target, "a.yaml", nil))
	link := filepath.Join(t.TempDir(), "processes.d")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, RemoveFiles(link, "a.yaml"))
	require.NoError(t, RemoveDirIfEmpty(link))
	assert.FileExists(t, filepath.Join(target, "a.yaml"))
}
