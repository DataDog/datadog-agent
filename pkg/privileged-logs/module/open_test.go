// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package module

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openPathWithoutSymlinksAndCheckFDs wraps openPathWithoutSymlinks and verifies
// that no file descriptors are leaked. For success cases, it checks that exactly
// one FD is opened. For error cases, it checks that no FDs are leaked.
func openPathWithoutSymlinksAndCheckFDs(t *testing.T, path string) (*os.File, error) {
	t.Helper()
	fdsBefore := countOpenFDs(t)

	file, err := openPathWithoutSymlinks(path)

	if err != nil {
		fdsAfter := countOpenFDs(t)
		assert.Equal(t, fdsBefore, fdsAfter, "FD count should not change on error")
		return nil, err
	}

	fdsAfter := countOpenFDs(t)
	assert.Equal(t, fdsBefore+1, fdsAfter, "should have exactly one more FD after opening")
	return file, nil
}

func TestOpenPathWithoutSymlinks(t *testing.T) {
	testDir := t.TempDir()

	t.Run("success - simple absolute path", func(t *testing.T) {
		logFile := filepath.Join(testDir, "test.log")
		err := os.WriteFile(logFile, []byte("test content"), 0644)
		require.NoError(t, err)

		file, err := openPathWithoutSymlinksAndCheckFDs(t, logFile)
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()

		buf := make([]byte, 12)
		n, err := file.Read(buf)
		require.NoError(t, err)
		assert.Equal(t, "test content", string(buf[:n]))
	})

	t.Run("success - nested path with multiple directories", func(t *testing.T) {
		nestedDir := filepath.Join(testDir, "a", "b", "c")
		err := os.MkdirAll(nestedDir, 0755)
		require.NoError(t, err)

		logFile := filepath.Join(nestedDir, "nested.log")
		err = os.WriteFile(logFile, []byte("nested content"), 0644)
		require.NoError(t, err)

		file, err := openPathWithoutSymlinksAndCheckFDs(t, logFile)
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()
	})

	t.Run("success - path with empty components", func(t *testing.T) {
		logFile := filepath.Join(testDir, "empty.log")
		err := os.WriteFile(logFile, []byte("test"), 0644)
		require.NoError(t, err)

		pathWithEmpty := testDir + "//empty.log"
		file, err := openPathWithoutSymlinksAndCheckFDs(t, pathWithEmpty)
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()
	})

	t.Run("success - can open directory", func(t *testing.T) {
		dir := filepath.Join(testDir, "justdir")
		err := os.Mkdir(dir, 0755)
		require.NoError(t, err)

		file, err := openPathWithoutSymlinksAndCheckFDs(t, dir)
		require.NoError(t, err)
		require.NotNil(t, file)
		defer file.Close()

		fi, err := file.Stat()
		require.NoError(t, err)
		assert.True(t, fi.IsDir())
	})

	t.Run("error - relative path", func(t *testing.T) {
		file, err := openPathWithoutSymlinksAndCheckFDs(t, "relative/path.log")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "path must be absolute")
		assert.Nil(t, file)
	})

	t.Run("error - non-existent path", func(t *testing.T) {
		nonExistentPath := filepath.Join(testDir, "nonexistent", "file.log")

		file, err := openPathWithoutSymlinksAndCheckFDs(t, nonExistentPath)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open directory component")
		assert.Nil(t, file)
	})

	t.Run("error - non-existent file in existing directory", func(t *testing.T) {
		nonExistentFile := filepath.Join(testDir, "nonexistent.log")

		file, err := openPathWithoutSymlinksAndCheckFDs(t, nonExistentFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
		assert.Nil(t, file)
	})

	t.Run("error - symlink in directory component", func(t *testing.T) {
		realDir := filepath.Join(testDir, "realdir")
		err := os.Mkdir(realDir, 0755)
		require.NoError(t, err)

		realFile := filepath.Join(realDir, "file.log")
		err = os.WriteFile(realFile, []byte("real content"), 0644)
		require.NoError(t, err)

		symlinkDir := filepath.Join(testDir, "symlinkdir")
		err = os.Symlink(realDir, symlinkDir)
		require.NoError(t, err)

		pathThroughSymlink := filepath.Join(symlinkDir, "file.log")

		file, err := openPathWithoutSymlinksAndCheckFDs(t, pathThroughSymlink)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open directory component")
		assert.Nil(t, file)
	})

	t.Run("error - symlink as final file component", func(t *testing.T) {
		realFile := filepath.Join(testDir, "realfile.log")
		err := os.WriteFile(realFile, []byte("real"), 0644)
		require.NoError(t, err)

		symlinkFile := filepath.Join(testDir, "symlinkfile.log")
		err = os.Symlink(realFile, symlinkFile)
		require.NoError(t, err)

		file, err := openPathWithoutSymlinksAndCheckFDs(t, symlinkFile)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
		assert.Nil(t, file)
	})

	t.Run("error - nested path with missing directory", func(t *testing.T) {
		nestedDir := filepath.Join(testDir, "nest1", "nest2")
		err := os.MkdirAll(nestedDir, 0755)
		require.NoError(t, err)

		nonExistentFile := filepath.Join(nestedDir, "nest3", "file.log")

		file, err := openPathWithoutSymlinksAndCheckFDs(t, nonExistentFile)
		assert.Error(t, err)
		assert.Nil(t, file)
	})
}
