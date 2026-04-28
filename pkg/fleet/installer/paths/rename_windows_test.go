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
	require.NoError(t, os.Mkdir(src, 0755))
	require.NoError(t, os.Mkdir(dst, 0755))

	err := Rename(src, dst)
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
	require.NoError(t, os.Mkdir(src, 0755))

	require.NoError(t, Rename(src, dst))
	assert.DirExists(t, dst)
	assert.NoDirExists(t, src)
}
