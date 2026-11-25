// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package filewriter

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setUmask gives the test the umask of most services, 022, under which a
// file gets the mode it is created with.
func setUmask(t *testing.T) {
	old := syscall.Umask(0o022)
	t.Cleanup(func() { syscall.Umask(old) })
}

// setLogDir makes dir the directory of the Agent logs for the test.
func setLogDir(t *testing.T, dir string) {
	old := logDir
	logDir = dir
	t.Cleanup(func() { logDir = old })
}

func TestRollingFileWriterFileMode(t *testing.T) {
	setUmask(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	logPath := filepath.Join(tempDir, "test.log")

	maxSize := int64(100)
	writer, err := NewRollingFileWriterSize(logPath, maxSize, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	// Write past the maximum size, then once more to roll the file.
	_, err = writer.Write([]byte(strings.Repeat("A", int(maxSize)+10)))
	require.NoError(t, err)
	_, err = writer.Write([]byte("trigger roll\n"))
	require.NoError(t, err)

	for _, path := range []string{logPath, logPath + ".1"} {
		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0640), info.Mode().Perm(), path)
	}
}

func TestRollingFileWriterTightensExistingFile(t *testing.T) {
	setUmask(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	logPath := filepath.Join(tempDir, "test.log")
	require.NoError(t, os.WriteFile(logPath, []byte("old\n"), 0644))

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("test\n"))
	require.NoError(t, err)

	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0640), info.Mode().Perm())
}

func TestRollingFileWriterFileModeOutsideLogDir(t *testing.T) {
	setUmask(t)
	tempDir := t.TempDir()
	created := filepath.Join(tempDir, "created.log")
	existing := filepath.Join(tempDir, "existing.log")
	require.NoError(t, os.WriteFile(existing, []byte("old\n"), 0644))

	for _, path := range []string{created, existing} {
		writer, err := NewRollingFileWriterSize(path, 1024, 5, RollingNameModePostfix)
		require.NoError(t, err)
		_, err = writer.Write([]byte("test\n"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		info, err := os.Stat(path)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0644), info.Mode().Perm(), path)
	}
}

func TestRollingFileWriterAgentGroup(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root")
	}
	// nobody, which most systems have, stands in for dd-agent.
	u, err := user.Lookup("nobody")
	if err != nil {
		t.Skip(err)
	}
	old := agentUser
	agentUser = u.Username
	t.Cleanup(func() { agentUser = old })
	tempDir := t.TempDir()
	setLogDir(t, tempDir)

	for _, tt := range []struct{ path, gid string }{
		{filepath.Join(tempDir, "test.log"), u.Gid},
		{filepath.Join(t.TempDir(), "test.log"), strconv.Itoa(os.Getegid())},
	} {
		writer, err := NewRollingFileWriterSize(tt.path, 1024, 5, RollingNameModePostfix)
		require.NoError(t, err)
		_, err = writer.Write([]byte("test\n"))
		require.NoError(t, err)
		require.NoError(t, writer.Close())

		info, err := os.Stat(tt.path)
		require.NoError(t, err)
		assert.Equal(t, tt.gid, strconv.Itoa(int(info.Sys().(*syscall.Stat_t).Gid)), tt.path)
	}
}

// setAgentUser makes nobody, which most systems have, stand in for
// dd-agent, so that a writer that runs as root would change the group of a
// file it handles.
func setAgentUser(t *testing.T) {
	u, err := user.Lookup("nobody")
	if err != nil {
		return
	}
	old := agentUser
	agentUser = u.Username
	t.Cleanup(func() { agentUser = old })
}

// statMode returns the permissions, group and link count of path.
func statMode(t *testing.T, path string) (os.FileMode, uint32, uint64) {
	info, err := os.Stat(path)
	require.NoError(t, err)
	st := info.Sys().(*syscall.Stat_t)
	return info.Mode().Perm(), st.Gid, uint64(st.Nlink)
}

func TestRollingFileWriterSymlinkKeepsTarget(t *testing.T) {
	setUmask(t)
	setAgentUser(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	target := filepath.Join(t.TempDir(), "target")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0644))
	mode, gid, _ := statMode(t, target)
	logPath := filepath.Join(tempDir, "test.log")
	require.NoError(t, os.Symlink(target, logPath))

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	_, err = writer.Write([]byte("test\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	afterMode, afterGid, _ := statMode(t, target)
	assert.Equal(t, mode, afterMode)
	assert.Equal(t, gid, afterGid)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, "old\ntest\n", string(data))
}

func TestRollingFileWriterDanglingSymlink(t *testing.T) {
	setUmask(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	target := filepath.Join(t.TempDir(), "target")
	logPath := filepath.Join(tempDir, "test.log")
	require.NoError(t, os.Symlink(target, logPath))

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	_, err = writer.Write([]byte("test\n"))
	assert.Error(t, err)
	require.NoError(t, writer.Close())

	_, err = os.Lstat(target)
	assert.True(t, os.IsNotExist(err), "the writer created %s", target)
}

func TestRollingFileWriterHardlinkKeepsMode(t *testing.T) {
	setUmask(t)
	setAgentUser(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	target := filepath.Join(tempDir, "target")
	require.NoError(t, os.WriteFile(target, []byte("old\n"), 0644))
	logPath := filepath.Join(tempDir, "test.log")
	require.NoError(t, os.Link(target, logPath))
	mode, gid, nlink := statMode(t, target)
	require.Equal(t, uint64(2), nlink)

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	_, err = writer.Write([]byte("test\n"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	afterMode, afterGid, _ := statMode(t, target)
	assert.Equal(t, mode, afterMode)
	assert.Equal(t, gid, afterGid)
}
