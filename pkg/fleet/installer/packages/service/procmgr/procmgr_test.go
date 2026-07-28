// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package procmgr

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCliPath(t *testing.T) {
	cli, err := cliPath("/opt/datadog-agent")
	require.NoError(t, err)
	assert.Equal(t, "/opt/datadog-agent/embedded/bin/dd-procmgr", cli)

	cli, err = cliPath("/opt/datadog-agent/")
	require.NoError(t, err)
	assert.Equal(t, "/opt/datadog-agent/embedded/bin/dd-procmgr", cli)

	for _, root := range []string{"", ".", "relative/root", "opt/datadog-agent"} {
		_, err := cliPath(root)
		assert.Error(t, err, "expected %q to be rejected", root)
	}
}

func TestWriteConfigCreatesDir(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteConfig(root, "datadog-agent-ddot.yaml", []byte("command: /bin/true\n")))

	dir, err := os.Stat(ConfigDir(root))
	require.NoError(t, err)
	assert.True(t, dir.IsDir())
	assert.Equal(t, os.FileMode(0755), dir.Mode().Perm())

	path := filepath.Join(ConfigDir(root), "datadog-agent-ddot.yaml")
	file, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), file.Mode().Perm())

	content, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "command: /bin/true\n", string(content))
}

func TestWriteConfigOverwrites(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteConfig(root, "a.yaml", []byte("first")))
	require.NoError(t, WriteConfig(root, "a.yaml", []byte("second")))

	content, err := os.ReadFile(filepath.Join(ConfigDir(root), "a.yaml"))
	require.NoError(t, err)
	assert.Equal(t, "second", string(content))
}

func TestRemoveConfigsIsIdempotent(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, WriteConfig(root, "a.yaml", []byte("x")))

	require.NoError(t, RemoveConfigs(root, "a.yaml", "never-existed.yaml"))
	require.NoError(t, RemoveConfigs(root, "a.yaml"))

	_, err := os.Stat(filepath.Join(ConfigDir(root), "a.yaml"))
	assert.True(t, os.IsNotExist(err))
}

func TestListConfigs(t *testing.T) {
	root := t.TempDir()

	names, err := ListConfigs(root)
	require.NoError(t, err, "a missing processes.d must not be an error")
	assert.Empty(t, names)

	require.NoError(t, WriteConfig(root, "b.yaml", []byte("x")))
	require.NoError(t, WriteConfig(root, "a.yml", []byte("x")))
	require.NoError(t, WriteConfig(root, "notes.txt", []byte("x")))
	require.NoError(t, os.MkdirAll(filepath.Join(ConfigDir(root), "subdir"), 0755))

	names, err = ListConfigs(root)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a.yml", "b.yaml"}, names)
}

func TestIsInstalled(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	installRoots = []string{other, root}
	t.Cleanup(func() { installRoots = nil })

	assert.False(t, IsInstalled())

	daemon := filepath.Join(root, daemonRelPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(daemon), 0755))
	require.NoError(t, os.WriteFile(daemon, []byte("#!/bin/true\n"), 0755))

	assert.True(t, IsInstalled())
}

func TestIsInstalledIgnoresDirectory(t *testing.T) {
	root := t.TempDir()
	installRoots = []string{root}
	t.Cleanup(func() { installRoots = nil })

	require.NoError(t, os.MkdirAll(filepath.Join(root, daemonRelPath), 0755))
	assert.False(t, IsInstalled())
}

func TestReloadWithoutDaemonIsNoOp(t *testing.T) {
	socketPath = filepath.Join(t.TempDir(), "absent.sock")
	t.Cleanup(func() { socketPath = "/var/run/datadog-procmgrd/dd-procmgrd.sock" })

	assert.NoError(t, Reload(context.Background(), "/opt/datadog-agent"))
}

func TestReloadWithoutCLIIsNoOp(t *testing.T) {
	dir := t.TempDir()
	socket := filepath.Join(dir, "dd-procmgrd.sock")
	require.NoError(t, os.WriteFile(socket, nil, 0600))
	socketPath = socket
	t.Cleanup(func() { socketPath = "/var/run/datadog-procmgrd/dd-procmgrd.sock" })

	assert.NoError(t, Reload(context.Background(), t.TempDir()))
}
