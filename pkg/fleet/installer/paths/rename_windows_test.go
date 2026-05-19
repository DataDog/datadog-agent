// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package paths

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestIsRetryableRenameError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"access denied", &os.LinkError{Op: "rename", Err: windows.ERROR_ACCESS_DENIED}, true},
		{"sharing violation", &os.LinkError{Op: "rename", Err: windows.ERROR_SHARING_VIOLATION}, true},
		{"file not found", &os.LinkError{Op: "rename", Err: windows.ERROR_FILE_NOT_FOUND}, false},
		{"path not found", &os.LinkError{Op: "rename", Err: windows.ERROR_PATH_NOT_FOUND}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableRenameError(tt.err))
		})
	}
}

func TestRename_TargetDirExists_ReturnsErrExist(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	require.NoError(t, os.Mkdir(src, 0o755))
	require.NoError(t, os.Mkdir(dst, 0o755))

	err := Rename(t.Context(), src, dst)
	require.ErrorIs(t, err, fs.ErrExist)

	var linkErr *os.LinkError
	require.ErrorAs(t, err, &linkErr)
	assert.Equal(t, "rename", linkErr.Op)
	assert.Equal(t, src, linkErr.Old)
	assert.Equal(t, dst, linkErr.New)

	// Source should still exist (we did not move it).
	assert.DirExists(t, src)
}

func TestRename_TargetMissing_Succeeds(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src")
	dst := filepath.Join(tmp, "dst")
	require.NoError(t, os.Mkdir(src, 0o755))

	require.NoError(t, Rename(t.Context(), src, dst))
	assert.DirExists(t, dst)
	assert.NoDirExists(t, src)
}

// setupLockedSrc creates `src` and `dst` paths in a fresh temp dir, with a
// file inside `src` held open. While the returned *os.File is live, Windows
// rename of `src` fails with a sharing violation. The caller is responsible
// for closing the handle (synchronously or asynchronously).
func setupLockedSrc(t *testing.T) (src, dst string, f *os.File) {
	t.Helper()
	tmp := t.TempDir()
	src = filepath.Join(tmp, "src")
	dst = filepath.Join(tmp, "dst")
	require.NoError(t, os.Mkdir(src, 0o755))
	lockPath := filepath.Join(src, "locked")
	require.NoError(t, os.WriteFile(lockPath, nil, 0o644))
	f, err := os.Open(lockPath)
	require.NoError(t, err)
	return src, dst, f
}

// TestRename_RetriesUntilLockReleased simulates the antimalware-scanner
// scenario: a file inside the source directory is held open, which causes
// os.Rename of the parent directory to fail with a sharing violation until
// the handle is closed.
//
// Uses testing/synctest so the backoff timer advances virtual time and the
// test does not actually wait through the retry budget.
func TestRename_RetriesUntilLockReleased(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, dst, f := setupLockedSrc(t)

		// Release the lock partway through the retry budget so a later
		// retry succeeds.
		go func() {
			time.Sleep(30 * time.Second)
			f.Close()
		}()

		assert.NoError(t, Rename(t.Context(), src, dst))
		assert.DirExists(t, dst)
		assert.NoDirExists(t, src)
	})
}

// TestRename_GivesUpWhenLockNeverReleased confirms the retry loop
// terminates after the elapsed-time budget rather than spinning forever
// when the underlying lock is never released. The error should still be
// the original ERROR_ACCESS_DENIED surfaced from os.Rename — Windows
// returns access-denied (not sharing-violation) when renaming a directory
// whose child file is held open without FILE_SHARE_DELETE.
func TestRename_GivesUpWhenLockNeverReleased(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		src, dst, f := setupLockedSrc(t)
		defer f.Close()

		err := Rename(t.Context(), src, dst)
		require.Error(t, err)
		assert.ErrorIs(t, err, windows.ERROR_ACCESS_DENIED,
			"expected ERROR_ACCESS_DENIED, got: %v", err)
		assert.DirExists(t, src, "source should still exist after failed rename")
		assert.NoDirExists(t, dst)
	})
}
