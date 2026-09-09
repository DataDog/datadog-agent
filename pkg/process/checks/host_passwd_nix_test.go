// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package checks

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePasswd(t *testing.T, dir, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "passwd"), []byte(contents), 0o600))
}

func newTestHostPasswdCache() (*hostPasswdCache, *clock.Mock) {
	clk := clock.NewMock()
	cache := newHostPasswdCache()
	cache.now = clk.Now
	return cache, clk
}

func TestHostPasswdLookup(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "host-user:x:100:200:Host User:/home/host:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cache, _ := newTestHostPasswdCache()

	u, found := cache.lookup("100")
	require.True(t, found)
	assert.Equal(t, "host-user", u.Username)
	assert.Equal(t, "200", u.Gid)

	// A caller must not be able to mutate the cached snapshot.
	u.Username = "modified"
	u, found = cache.lookup("100")
	require.True(t, found)
	assert.Equal(t, "host-user", u.Username)

	u, found = cache.lookup("101")
	assert.False(t, found)
	assert.Nil(t, u)
}

func TestHostPasswdReadsHostEtcLazily(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "late-user:x:123:123::/:/bin/sh\n")
	t.Setenv("HOST_ETC", "")
	cache, _ := newTestHostPasswdCache()
	_, found := cache.lookup("123")
	require.False(t, found)

	// A path change takes effect even before the next refresh interval.
	t.Setenv("HOST_ETC", dir)
	u, found := cache.lookup("123")
	require.True(t, found)
	assert.Equal(t, "late-user", u.Username)

	t.Setenv("HOST_ETC", "")
	_, found = cache.lookup("123")
	assert.False(t, found)
}

func TestHostPasswdSkipsInvalidRowsAndFirstDuplicateWins(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "\n# comment\nmalformed\n+compat:x:1:1::/:/bin/sh\n-invalid:x:2:2::/:/bin/sh\nbaduid:x:nope:3::/:/bin/sh\nfirst:x:42:4::/:/bin/sh\nsecond:x:42:5::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cache, _ := newTestHostPasswdCache()
	u, found := cache.lookup("42")
	require.True(t, found)
	assert.Equal(t, "first", u.Username)
	assert.Equal(t, "4", u.Gid)

	_, found = cache.lookup("1")
	assert.False(t, found)
}

func TestHostPasswdRefresh(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "before:x:77:77::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cache, clk := newTestHostPasswdCache()
	u, found := cache.lookup("77")
	require.True(t, found)
	require.Equal(t, "before", u.Username)

	writePasswd(t, dir, "changed-user:x:77:77::/:/bin/sh\n")
	clk.Add(hostPasswdRefreshInterval - time.Nanosecond)
	u, found = cache.lookup("77")
	require.True(t, found)
	assert.Equal(t, "before", u.Username, "must not reload before the interval")

	clk.Add(time.Nanosecond)
	u, found = cache.lookup("77")
	require.True(t, found)
	assert.Equal(t, "changed-user", u.Username)

	// Same size and mtime, but a different inode: identity must trigger reload.
	passwdPath := filepath.Join(dir, "passwd")
	info, err := os.Stat(passwdPath)
	require.NoError(t, err)
	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(replacement, []byte("another-user:x:77:77::/:/bin/sh\n"), 0o600))
	require.NoError(t, os.Chtimes(replacement, info.ModTime(), info.ModTime()))
	require.NoError(t, os.Rename(replacement, passwdPath))
	clk.Add(hostPasswdRefreshInterval)
	u, found = cache.lookup("77")
	require.True(t, found)
	assert.Equal(t, "another-user", u.Username)
}

func TestHostPasswdFailedRefreshKeepsLastCompleteSnapshot(t *testing.T) {
	dir := t.TempDir()
	passwdPath := filepath.Join(dir, "passwd")
	writePasswd(t, dir, "last-good:x:88:88::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cache, clk := newTestHostPasswdCache()
	_, found := cache.lookup("88")
	require.True(t, found)

	require.NoError(t, os.Remove(passwdPath))
	require.NoError(t, os.Mkdir(passwdPath, 0o700))
	clk.Add(hostPasswdRefreshInterval)
	u, found := cache.lookup("88")
	require.True(t, found)
	assert.Equal(t, "last-good", u.Username)
}

func TestHostPasswdMissingFileAndRecovery(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOST_ETC", dir)
	cache, clk := newTestHostPasswdCache()
	_, found := cache.lookup("88")
	require.False(t, found)

	writePasswd(t, dir, "recovered:x:88:88::/:/bin/sh\n")
	clk.Add(hostPasswdRefreshInterval)
	u, found := cache.lookup("88")
	require.True(t, found)
	assert.Equal(t, "recovered", u.Username)

	require.NoError(t, os.Remove(filepath.Join(dir, "passwd")))
	clk.Add(hostPasswdRefreshInterval)
	_, found = cache.lookup("88")
	assert.False(t, found)
}

func TestHostPasswdConcurrentRefresh(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "before:x:99:99::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	cache, clk := newTestHostPasswdCache()
	_, found := cache.lookup("99")
	require.True(t, found)

	var wg sync.WaitGroup
	stop := make(chan struct{})
	errCh := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				u, found := cache.lookup("99")
				if !found || (u.Username != "before" && u.Username != "after") {
					errCh <- fmt.Errorf("unexpected snapshot: user=%v found=%v", u, found)
					return
				}
			}
		}()
	}
	defer func() {
		close(stop)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			assert.NoError(t, err)
		}
	}()

	for range 10 {
		for _, name := range []string{"after", "before"} {
			replacement := filepath.Join(dir, "replacement")
			require.NoError(t, os.WriteFile(replacement, []byte(name+":x:99:99::/:/bin/sh\n"), 0o600))
			require.NoError(t, os.Rename(replacement, filepath.Join(dir, "passwd")))
			clk.Add(hostPasswdRefreshInterval)
			u, found := cache.lookup("99")
			require.True(t, found)
			assert.Equal(t, name, u.Username)
		}
	}
}
