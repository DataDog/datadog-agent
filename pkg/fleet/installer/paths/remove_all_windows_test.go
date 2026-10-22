// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package paths

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/cenkalti/backoff/v7"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

func TestIsRetryableRemoveAllError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"access denied", &os.PathError{Op: "remove", Err: windows.ERROR_ACCESS_DENIED}, true},
		{"sharing violation", &os.PathError{Op: "remove", Err: windows.ERROR_SHARING_VIOLATION}, true},
		{"directory not empty", &os.PathError{Op: "remove", Err: windows.ERROR_DIR_NOT_EMPTY}, true},
		{"file not found", &os.PathError{Op: "remove", Err: windows.ERROR_FILE_NOT_FOUND}, false},
		{"invalid path", &os.PathError{Op: "remove", Err: windows.ERROR_INVALID_NAME}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isRetryableRemoveAllError(tt.err))
		})
	}
}

func TestRemoveAllRetriesThenSucceeds(t *testing.T) {
	attempts := 0
	err := removeAll(t.Context(), func() error {
		attempts++
		if attempts < 3 {
			return fs.ErrPermission
		}
		return nil
	},
		backoff.WithBackOff(&backoff.ZeroBackOff{}),
		backoff.WithMaxTries(3),
	)
	require.NoError(t, err)
	assert.Equal(t, 3, attempts)
}

func TestRemoveAllReturnsErrorAfterRetries(t *testing.T) {
	attempts := 0
	removeErr := fs.ErrPermission
	err := removeAll(t.Context(), func() error {
		attempts++
		return removeErr
	},
		backoff.WithBackOff(&backoff.ZeroBackOff{}),
		backoff.WithMaxTries(3),
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, removeErr)
	assert.ErrorIs(t, err, backoff.ErrExhausted)
	assert.Equal(t, 3, attempts)
}

func TestRemoveAllDoesNotRetryPermanentError(t *testing.T) {
	attempts := 0
	removeErr := errors.New("permanent error")
	err := removeAll(t.Context(), func() error {
		attempts++
		return removeErr
	},
		backoff.WithBackOff(&backoff.ZeroBackOff{}),
		backoff.WithMaxTries(3),
	)
	require.ErrorIs(t, err, removeErr)
	assert.Equal(t, 1, attempts)
}

func TestRemoveAllContextCancellationInterruptsBackoff(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		attempts := 0
		go func() {
			time.Sleep(time.Second)
			cancel()
		}()

		err := removeAll(ctx, func() error {
			attempts++
			return fs.ErrPermission
		}, backoff.WithBackOff(backoff.NewConstantBackOff(time.Minute)))
		require.Error(t, err)
		assert.ErrorIs(t, err, context.Canceled)
		assert.Equal(t, 1, attempts)
	})
}

// TestRemoveAllRetriesUntilLockReleased exercises the real Windows filesystem
// behavior that motivates the retry: an open file handle temporarily prevents
// its parent directory from being removed.
func TestRemoveAllRetriesUntilLockReleased(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "remove-me")
		require.NoError(t, os.Mkdir(dir, 0o755))
		lockedPath := filepath.Join(dir, "locked")
		require.NoError(t, os.WriteFile(lockedPath, nil, 0o644))
		lockedFile, err := os.Open(lockedPath)
		require.NoError(t, err)

		go func() {
			time.Sleep(30 * time.Second)
			_ = lockedFile.Close()
		}()

		require.NoError(t, RemoveAll(t.Context(), dir))
		assert.NoDirExists(t, dir)
	})
}
