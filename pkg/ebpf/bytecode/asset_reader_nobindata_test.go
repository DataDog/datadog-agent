// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !ebpf_bindata

package bytecode

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requireRootOwnedAssetDir returns a temp dir usable for assets that must pass
// VerifyAssetPermissionsAndOpen. Since that check demands root:root ownership,
// the caller has to be root.
func requireRootOwnedAssetDir(t *testing.T) string {
	t.Helper()

	if os.Geteuid() != 0 {
		t.Skip("test must run as root: assets are required to be root-owned")
	}
	return t.TempDir()
}

func TestGetReaderPrefersUncompressedAsset(t *testing.T) {
	dir := requireRootOwnedAssetDir(t)

	plain := []byte("uncompressed asset")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o"), plain, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o.gz"), gzipBytes(t, []byte("compressed asset")), 0644))

	reader, err := GetReader(dir, "asset.o")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, plain, got)
}

func TestGetReaderFallsBackToCompressedAsset(t *testing.T) {
	dir := requireRootOwnedAssetDir(t)

	content := testAssetContent()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o.gz"), gzipBytes(t, content), 0644))

	reader, err := GetReader(dir, "asset.o")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestGetReaderFallsBackToXZAsset(t *testing.T) {
	dir := requireRootOwnedAssetDir(t)

	compressed, err := os.ReadFile(filepath.Join("testdata", "asset.xz"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o.xz"), compressed, 0644))

	reader, err := GetReader(dir, "asset.o")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, reader.Close()) })

	got, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Equal(t, testAssetContent(), got)
}

// A corrupt compressed asset must surface as an error rather than be silently
// skipped in favor of the next candidate extension.
func TestGetReaderCorruptedCompressedAsset(t *testing.T) {
	dir := requireRootOwnedAssetDir(t)

	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o.gz"), []byte("not a gzip stream"), 0644))

	reader, err := GetReader(dir, "asset.o")
	if reader != nil {
		reader.Close()
	}
	assert.ErrorContains(t, err, "asset.o.gz")
}

// When no variant exists the error must name the plain path: that is what
// callers ask for and what troubleshooting docs reference.
func TestGetReaderMissingAssetReportsPlainPath(t *testing.T) {
	dir := t.TempDir()

	reader, err := GetReader(dir, "asset.o")
	if reader != nil {
		reader.Close()
	}
	require.ErrorIs(t, err, fs.ErrNotExist)
	assert.ErrorContains(t, err, filepath.Join(dir, "asset.o"))
}

// The complement of the root-only tests above: as non-root the compressed
// candidate is reached but rejected by the ownership check, which proves
// GetReader probes "<name>.gz" and surfaces its permission error instead of
// silently falling through to the "asset missing" error.
func TestGetReaderCompressedAssetPermissionsEnforced(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must run as non-root to observe the ownership rejection")
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "asset.o.gz"), gzipBytes(t, testAssetContent()), 0644))

	reader, err := GetReader(dir, "asset.o")
	if reader != nil {
		reader.Close()
	}
	require.Error(t, err)
	assert.NotErrorIs(t, err, fs.ErrNotExist, "the compressed asset exists; the error must be the permission rejection")
	assert.ErrorContains(t, err, "asset.o.gz")
}
