// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package ntfsdu_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// scanResult is a minimal projection of the JSON document for assertions.
type scanResult struct {
	Target       string `json:"target"`
	Mode         string `json:"mode"`
	SubtreeBytes int64  `json:"subtreeBytes"`
	Tree         []struct {
		Path        string `json:"path"`
		SizeBytes   int64  `json:"sizeBytes"`
		Pruned      bool   `json:"pruned"`
		FileCount   int    `json:"fileCount"`
		FolderCount int    `json:"folderCount"`
	} `json:"tree"`
	TopFiles []struct {
		Path      string `json:"path"`
		SizeBytes int64  `json:"sizeBytes"`
		Created   string `json:"created"`
		Modified  string `json:"modified"`
	} `json:"topFiles"`
}

// TestScanTempDirJSON opportunistically validates the full builtin→engine→JSON
// path against a temp directory on the current volume. A real scan needs a
// genuine NTFS volume opened with elevation; environments that can't provide
// that are skipped rather than failed:
//   - containers (including Windows CI containers) expose C: as a filesystem
//     layer, not a raw volume, so raw $MFT reads fail with ERROR_NOT_SUPPORTED /
//     ERROR_INVALID_FUNCTION ("not supported" / "incorrect function");
//   - non-elevated processes are denied the volume handle ("access is denied").
//
// Because CI cannot reliably exercise the scan, this test is best-effort only.
// Real validation of scan correctness belongs in a VM-based E2E test (see the
// Testing note in AGENTS.md).
func TestScanTempDirJSON(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, 64*1024), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "small.txt"), []byte("hello"), 0o644))

	// Single-quote the path: rshell is a POSIX shell, so the backslashes in a
	// Windows path would otherwise be consumed as escape characters.
	stdout, stderr, code := testutil.RunScript(t,
		"ntfs-du --output json --top-files 5 '"+dir+"'", dir, interp.AllowedPaths([]string{dir}))

	if code != 0 {
		low := strings.ToLower(stderr)
		if strings.Contains(low, "access is denied") || strings.Contains(low, "need admin") ||
			strings.Contains(low, "not supported") || strings.Contains(low, "incorrect function") {
			t.Skipf("ntfs-du scan unavailable in this environment: %s", strings.TrimSpace(stderr))
		}
		t.Fatalf("ntfs-du failed (code %d): %s", code, stderr)
	}

	var res scanResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	assert.Equal(t, "allocated", res.Mode)
	assert.NotEmpty(t, res.Target)
	require.NotEmpty(t, res.Tree, "flattened tree should have at least the root node at default depth 1")
	assert.NotEmpty(t, res.Tree[0].Path, "root tree node should carry the target path")
	assert.GreaterOrEqual(t, res.SubtreeBytes, int64(64*1024), "subtree should include the 64 KiB file")
	// The temp dir holds exactly two files and no subfolders; counts are not
	// filtered by --min, so the root node reports them in full.
	assert.Equal(t, 2, res.Tree[0].FileCount, "root fileCount")
	assert.Equal(t, 0, res.Tree[0].FolderCount, "root folderCount")
}

// top-files entries carry RFC 3339 created/modified timestamps, read from the
// file handle during post-scan path resolution.
func TestScanTopFilesTimestamps(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.bin"), make([]byte, 256*1024), 0o644))

	stdout, stderr, code := testutil.RunScript(t,
		"ntfs-du --output json --apparent-size --min 0 --max-depth 0 --top-files 5 '"+dir+"'",
		dir, interp.AllowedPaths([]string{dir}))
	if code != 0 {
		low := strings.ToLower(stderr)
		if strings.Contains(low, "access is denied") || strings.Contains(low, "need admin") ||
			strings.Contains(low, "not supported") || strings.Contains(low, "incorrect function") {
			t.Skipf("ntfs-du scan unavailable in this environment: %s", strings.TrimSpace(stderr))
		}
		t.Fatalf("ntfs-du failed (code %d): %s", code, stderr)
	}

	var res scanResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	require.NotEmpty(t, res.TopFiles, "big.bin should appear in top-files at --min 0")
	f := res.TopFiles[0]
	assert.NotEmpty(t, f.Created, "top-file should carry a created timestamp")
	assert.NotEmpty(t, f.Modified, "top-file should carry a modified timestamp")
	_, err := time.Parse(time.RFC3339, f.Created)
	assert.NoError(t, err, "created must be RFC 3339: %q", f.Created)
	_, err = time.Parse(time.RFC3339, f.Modified)
	assert.NoError(t, err, "modified must be RFC 3339: %q", f.Modified)
}

// At --max-depth 0 the folder tree is omitted entirely (the output carries only
// totals and the top files/extensions).
func TestScanDepthZeroOmitsTree(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.bin"), make([]byte, 4096), 0o644))

	stdout, stderr, code := testutil.RunScript(t,
		"ntfs-du --output json --min 0 --max-depth 0 '"+dir+"'", dir, interp.AllowedPaths([]string{dir}))
	if code != 0 {
		low := strings.ToLower(stderr)
		if strings.Contains(low, "access is denied") || strings.Contains(low, "need admin") ||
			strings.Contains(low, "not supported") || strings.Contains(low, "incorrect function") {
			t.Skipf("ntfs-du scan unavailable in this environment: %s", strings.TrimSpace(stderr))
		}
		t.Fatalf("ntfs-du failed (code %d): %s", code, stderr)
	}
	assert.NotContains(t, stdout, `"tree"`, "tree key must be omitted at --max-depth 0")
	var res scanResult
	require.NoError(t, json.Unmarshal([]byte(stdout), &res))
	assert.Empty(t, res.Tree, "no tree at --max-depth 0")
}
