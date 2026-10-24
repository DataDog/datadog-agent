// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// TestNowConsistentAcrossRoots verifies that find with multiple root paths
// finds files in all roots when using a shared time reference from
// CallContext.Now. The single-capture invariant is now structurally enforced
// by the time.Time value field (there is no function to call multiple times),
// so this test acts as a regression guard for multi-root traversal with a
// time predicate.
func TestNowConsistentAcrossRoots(t *testing.T) {
	// Create two directories with one file each.
	tmp := t.TempDir()
	dir1 := filepath.Join(tmp, "a")
	dir2 := filepath.Join(tmp, "b")
	require.NoError(t, os.MkdirAll(dir1, 0755))
	require.NoError(t, os.MkdirAll(dir2, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir1, "f1.txt"), []byte("x"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "f2.txt"), []byte("y"), 0644))

	var stdout, stderr bytes.Buffer
	callCtx := &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    time.Now(),
		LstatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
			return os.Lstat(filepath.Join(tmp, path))
		},
		StatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
			return os.Stat(filepath.Join(tmp, path))
		},
		OpenDir: func(_ context.Context, path string) (fs.ReadDirFile, error) {
			return os.Open(filepath.Join(tmp, path))
		},
		IsDirEmpty: func(_ context.Context, path string) (bool, error) {
			entries, err := os.ReadDir(filepath.Join(tmp, path))
			if err != nil {
				return false, err
			}
			return len(entries) == 0, nil
		},
		PortableErr: func(err error) string {
			return err.Error()
		},
	}

	// Run find with two root paths and a time predicate.
	result := run(context.Background(), callCtx, []string{"a", "b", "-mmin", "-60"})

	assert.Equal(t, uint8(0), result.Code, "find should succeed")
	assert.Contains(t, stdout.String(), "f1.txt")
	assert.Contains(t, stdout.String(), "f2.txt")
}

// TestNowFromCallContextIsUsed verifies that find actually uses the Now value
// from CallContext for predicate evaluation. A fixed timestamp far in the
// future is supplied; files created right now should appear very old relative
// to that future Now, so they should match +1 (older than 1 minute) and not
// match -1 (newer than 1 minute).
func TestNowFromCallContextIsUsed(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(tmp, "fresh.txt"), []byte("x"), 0644))

	var stdout, stderr bytes.Buffer
	// Use a timestamp 10 years in the future. From that reference point the
	// fresh file was created "10 years ago", so diff = futureNow - mtime ≈ 10yr.
	// It should match +1 (older than 1 minute) but not -1 (newer than 1 minute).
	futureNow := time.Now().Add(10 * 365 * 24 * time.Hour)
	callCtx := &builtins.CallContext{
		Stdout: &stdout,
		Stderr: &stderr,
		Now:    futureNow,
		LstatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
			return os.Lstat(filepath.Join(tmp, path))
		},
		StatFile: func(_ context.Context, path string) (fs.FileInfo, error) {
			return os.Stat(filepath.Join(tmp, path))
		},
		OpenDir: func(_ context.Context, path string) (fs.ReadDirFile, error) {
			return os.Open(filepath.Join(tmp, path))
		},
		IsDirEmpty: func(_ context.Context, path string) (bool, error) {
			entries, err := os.ReadDir(filepath.Join(tmp, path))
			if err != nil {
				return false, err
			}
			return len(entries) == 0, nil
		},
		PortableErr: func(err error) string {
			return err.Error()
		},
	}

	// A fresh file should match +1 (older than 1 minute) when Now is 10 years
	// in the future, proving that CallContext.Now is used for evaluation.
	result := run(context.Background(), callCtx, []string{".", "-name", "fresh.txt", "-mmin", "+1"})
	assert.Equal(t, uint8(0), result.Code, "find should succeed")
	assert.Contains(t, stdout.String(), "fresh.txt",
		"fresh file should match -mmin +1 when CallContext.Now is 10 years in the future")
	assert.Empty(t, stderr.String())

	// The same file should NOT match -1 (newer than 1 minute) under the same Now.
	stdout.Reset()
	stderr.Reset()
	result = run(context.Background(), callCtx, []string{".", "-name", "fresh.txt", "-mmin", "-1"})
	assert.Equal(t, uint8(0), result.Code, "find should succeed")
	assert.NotContains(t, stdout.String(), "fresh.txt",
		"fresh file should not match -mmin -1 when CallContext.Now is 10 years in the future")
	assert.Empty(t, stderr.String())
}
