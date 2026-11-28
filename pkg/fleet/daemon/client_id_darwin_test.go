// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
)

func TestLoadOrCreateClientIDCreatesTheFileOnce(t *testing.T) {
	dir := t.TempDir()

	first, err := loadOrCreateClientID(dir)
	require.NoError(t, err)
	assert.True(t, client.ValidClientID(first))

	second, err := loadOrCreateClientID(dir)
	require.NoError(t, err)
	assert.Equal(t, first, second, "a later daemon reads the ID the first one created")

	content, err := os.ReadFile(filepath.Join(dir, clientIDFile))
	require.NoError(t, err)
	assert.Equal(t, first, string(content))
	info, err := os.Stat(filepath.Join(dir, clientIDFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestLoadOrCreateClientIDReplacesAnInvalidFile(t *testing.T) {
	for name, content := range map[string]string{
		"empty":            "",
		"too short":        "abc",
		"too long":         strings.Repeat("a", 64),
		"a newline":        strings.Repeat("a", 20) + "\n",
		"bad character":    strings.Repeat("a", 20) + "/",
		"an ID and a tail": client.GenerateClientID() + "x",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, clientIDFile)
			require.NoError(t, os.WriteFile(path, []byte(content), 0644))

			id, err := loadOrCreateClientID(dir)
			require.NoError(t, err)
			assert.True(t, client.ValidClientID(id))

			again, err := loadOrCreateClientID(dir)
			require.NoError(t, err)
			assert.Equal(t, id, again)
			stored, err := os.ReadFile(path)
			require.NoError(t, err)
			assert.Equal(t, id, string(stored))
		})
	}
}

func TestLoadOrCreateClientIDDoesNotFollowALink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	other := client.GenerateClientID()
	require.NoError(t, os.WriteFile(target, []byte(other), 0644))
	require.NoError(t, os.Symlink(target, filepath.Join(dir, clientIDFile)))

	id, err := loadOrCreateClientID(dir)
	require.NoError(t, err)

	assert.NotEqual(t, other, id, "the ID behind a link is not trusted")
	assert.True(t, client.ValidClientID(id))
	info, err := os.Lstat(filepath.Join(dir, clientIDFile))
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular(), "the link is replaced by a regular file")
	untouched, err := os.ReadFile(target)
	require.NoError(t, err)
	assert.Equal(t, other, string(untouched), "what the link pointed at is left alone")
}

func TestLoadOrCreateClientIDIgnoresADirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, clientIDFile), 0755))

	_, err := loadOrCreateClientID(dir)
	assert.Error(t, err, "a directory cannot be replaced by the ID file")
}

func TestLoadOrCreateClientIDConcurrentDaemonsAgree(t *testing.T) {
	dir := t.TempDir()
	const n = 16
	ids := make([]string, n)
	errs := make([]error, n)
	var ready, done sync.WaitGroup
	start := make(chan struct{})
	ready.Add(n)
	done.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			ids[i], errs[i] = loadOrCreateClientID(dir)
		}()
	}
	ready.Wait()
	close(start)
	done.Wait()

	for i := range ids {
		require.NoError(t, errs[i])
		assert.Equal(t, ids[0], ids[i], "every daemon ends up with the ID of the one that created the file")
	}
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1, "no temporary file is left behind")
}

func TestLoadOrCreateClientIDFailsInAMissingDirectory(t *testing.T) {
	_, err := loadOrCreateClientID(filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
}

func TestLoadOrCreateClientIDFailsInAReadOnlyDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root writes to a read-only directory")
	}
	dir := t.TempDir()
	require.NoError(t, os.Chmod(dir, 0555))
	t.Cleanup(func() { _ = os.Chmod(dir, 0755) })

	_, err := loadOrCreateClientID(dir)
	assert.Error(t, err)
}
