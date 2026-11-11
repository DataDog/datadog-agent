// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package allowedpaths

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxStatFSWindowsRootRelativeOperand(t *testing.T) {
	dir := t.TempDir()
	cwd := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(cwd, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target"), []byte("data"), 0o600))

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	absoluteTarget := filepath.Join(dir, "target")
	rootRelativeTarget := strings.TrimPrefix(absoluteTarget, filepath.VolumeName(absoluteTarget))
	_, err = sb.StatFS(rootRelativeTarget, cwd)
	require.NoError(t, err)
}

func TestSandboxStatFSWindowsRootRelativeSymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	target := filepath.Join(dir, "target")
	require.NoError(t, os.Mkdir(subdir, 0o700))
	require.NoError(t, os.WriteFile(target, []byte("data"), 0o600))

	rootRelativeTarget := strings.TrimPrefix(target, filepath.VolumeName(target))
	if err := os.Symlink(rootRelativeTarget, filepath.Join(subdir, "link")); err != nil {
		t.Skipf("symlink creation is unavailable: %v", err)
	}

	sb, _, err := New([]string{dir})
	require.NoError(t, err)
	defer sb.Close()

	_, err = sb.StatFS(filepath.Join("subdir", "link"), dir)
	require.NoError(t, err)
}
