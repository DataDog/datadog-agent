// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package headers

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

type attemptBackend struct{ run func(string) error }

func (b attemptBackend) GetKernelHeaders(dir string) error { return b.run(dir) }
func (attemptBackend) Close()                              {}

// TestDownloadAttemptCleansFailures checks that stale output is removed before
// a download, rejected partial output is removed after failure, outside symlink
// targets are untouched, and the next attempt starts with a clean destination.
func TestDownloadAttemptCleansFailures(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("untouched"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stale"), nil, 0600))
	failure := errors.New("rejected archive")
	backend := attemptBackend{run: func(output string) error {
		entries, err := os.ReadDir(output)
		require.NoError(t, err)
		require.Empty(t, entries)
		require.NoError(t, os.WriteFile(filepath.Join(output, "partial.h"), nil, 0600))
		require.NoError(t, os.Symlink(outside, filepath.Join(output, "escape")))
		return failure
	}}
	require.ErrorIs(t, downloadAttempt(backend, dir), failure)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entries)
	content, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	require.Equal(t, "untouched", string(content))
	backend.run = func(output string) error { return os.WriteFile(filepath.Join(output, "valid.h"), nil, 0600) }
	require.NoError(t, downloadAttempt(backend, dir))
	require.FileExists(t, filepath.Join(dir, "valid.h"))
}
