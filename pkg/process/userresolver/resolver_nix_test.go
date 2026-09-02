// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package userresolver

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writePasswd(t *testing.T, dir, contents string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "passwd"), []byte(contents), 0o600))
}

func newTestResolver(fallback LookupIDFunc) (*Resolver, *time.Time) {
	now := time.Unix(100, 0)
	resolver := New(fallback)
	resolver.now = func() time.Time { return now }
	return resolver, &now
}

func TestLookupIDHostPrecedenceAndFallback(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "host-user:x:100:200:Host User:/home/host:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	var fallbackCalls atomic.Int32
	resolver, _ := newTestResolver(func(uid string) (*user.User, error) {
		fallbackCalls.Add(1)
		return &user.User{Username: "local-user", Uid: uid}, nil
	})

	hostUser, err := resolver.LookupID("100")
	require.NoError(t, err)
	assert.Equal(t, "host-user", hostUser.Username)
	assert.Equal(t, "200", hostUser.Gid)
	assert.Equal(t, int32(0), fallbackCalls.Load())

	localUser, err := resolver.LookupID("101")
	require.NoError(t, err)
	assert.Equal(t, "local-user", localUser.Username)
	assert.Equal(t, int32(1), fallbackCalls.Load())
}

func TestLookupIDReadsHostEtcLazily(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "late-user:x:123:123::/:/bin/sh\n")
	t.Setenv("HOST_ETC", "")

	resolver, now := newTestResolver(func(uid string) (*user.User, error) {
		return &user.User{Username: "fallback", Uid: uid}, nil
	})
	got, err := resolver.LookupID("123")
	require.NoError(t, err)
	assert.Equal(t, "fallback", got.Username)

	t.Setenv("HOST_ETC", dir)
	*now = now.Add(time.Millisecond)
	got, err = resolver.LookupID("123")
	require.NoError(t, err)
	assert.Equal(t, "late-user", got.Username)
}

func TestLookupIDSkipsInvalidPasswdRowsAndFirstDuplicateWins(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "\n# comment\nmalformed\n+compat:x:1:1::/:/bin/sh\n-invalid:x:2:2::/:/bin/sh\nbaduid:x:nope:3::/:/bin/sh\nfirst:x:42:4::/:/bin/sh\nsecond:x:42:5::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	resolver, _ := newTestResolver(func(string) (*user.User, error) {
		return nil, errors.New("unknown user")
	})
	got, err := resolver.LookupID("42")
	require.NoError(t, err)
	assert.Equal(t, "first", got.Username)
	assert.Equal(t, "4", got.Gid)

	_, err = resolver.LookupID("1")
	assert.Error(t, err)
}

func TestLookupIDRefreshesChangedAndReplacedFiles(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "before:x:77:77::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	resolver, now := newTestResolver(func(uid string) (*user.User, error) {
		return &user.User{Username: "fallback", Uid: uid}, nil
	})
	got, err := resolver.LookupID("77")
	require.NoError(t, err)
	assert.Equal(t, "before", got.Username)

	writePasswd(t, dir, "changed-user:x:77:77::/:/bin/sh\n")
	*now = now.Add(defaultRefreshInterval)
	got, err = resolver.LookupID("77")
	require.NoError(t, err)
	assert.Equal(t, "changed-user", got.Username)

	replacement := filepath.Join(dir, "replacement")
	require.NoError(t, os.WriteFile(replacement, []byte("replacement:x:77:77::/:/bin/sh\n"), 0o600))
	require.NoError(t, os.Rename(replacement, filepath.Join(dir, "passwd")))
	*now = now.Add(defaultRefreshInterval)
	got, err = resolver.LookupID("77")
	require.NoError(t, err)
	assert.Equal(t, "replacement", got.Username)
}

func TestLookupIDFailedRefreshKeepsLastCompleteSnapshot(t *testing.T) {
	dir := t.TempDir()
	passwdPath := filepath.Join(dir, "passwd")
	writePasswd(t, dir, "last-good:x:88:88::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)

	fallbackErr := errors.New("fallback failed")
	resolver, now := newTestResolver(func(string) (*user.User, error) { return nil, fallbackErr })
	got, err := resolver.LookupID("88")
	require.NoError(t, err)
	require.Equal(t, "last-good", got.Username)

	require.NoError(t, os.Remove(passwdPath))
	require.NoError(t, os.Mkdir(passwdPath, 0o700))
	*now = now.Add(defaultRefreshInterval)
	got, err = resolver.LookupID("88")
	require.NoError(t, err)
	assert.Equal(t, "last-good", got.Username)
}

func TestLookupIDMissingFileFallsBackAndRecovers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOST_ETC", dir)
	fallbackErr := errors.New("fallback failed")
	resolver, now := newTestResolver(func(string) (*user.User, error) { return nil, fallbackErr })

	passwdPath := filepath.Join(dir, "passwd")
	require.NoError(t, os.Mkdir(passwdPath, 0o700))
	_, err := resolver.LookupID("88")
	assert.ErrorIs(t, err, fallbackErr)
	require.NoError(t, os.Remove(passwdPath))

	writePasswd(t, dir, "recovered:x:88:88::/:/bin/sh\n")
	*now = now.Add(defaultRefreshInterval)
	got, err := resolver.LookupID("88")
	require.NoError(t, err)
	assert.Equal(t, "recovered", got.Username)

	require.NoError(t, os.Remove(passwdPath))
	*now = now.Add(defaultRefreshInterval)
	_, err = resolver.LookupID("88")
	assert.ErrorIs(t, err, fallbackErr)
}

func TestLookupIDConcurrent(t *testing.T) {
	dir := t.TempDir()
	writePasswd(t, dir, "concurrent:x:99:99::/:/bin/sh\n")
	t.Setenv("HOST_ETC", dir)
	resolver := New(func(uid string) (*user.User, error) {
		return &user.User{Username: "fallback", Uid: uid}, nil
	})

	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				got, err := resolver.LookupID("99")
				if err != nil {
					errCh <- err
					return
				}
				if got.Username != "concurrent" {
					errCh <- fmt.Errorf("unexpected username %q", got.Username)
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}
}
