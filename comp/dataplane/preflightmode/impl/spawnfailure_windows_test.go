// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package preflightmodeimpl

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifySpawnErrorWindowsCodes covers the Windows error codes that have no fs sentinel.
func TestClassifySpawnErrorWindowsCodes(t *testing.T) {
	pathErr := func(errno syscall.Errno) error {
		return &fs.PathError{Op: "open", Path: `C:\ProgramData\Datadog\run\adp-preflight`, Err: errno}
	}
	tests := []struct {
		errno syscall.Errno
		want  spawnErrorClass
	}{
		{errorWriteProtect, spawnErrorReadOnly},
		{errorHandleDiskFull, spawnErrorNoSpace},
		{errorDiskFull, spawnErrorNoSpace},
		{errorDiskQuotaExceeded, spawnErrorNoSpace},
		{errorBadExeFormat, spawnErrorExecFormat},
		{errorExeMachineTypeMismatch, spawnErrorExecFormat},
		{errorFilenameExcedRange, spawnErrorNameTooLong},
		{syscall.ERROR_ACCESS_DENIED, spawnErrorPermissionDenied},
		{syscall.ERROR_FILE_NOT_FOUND, spawnErrorNotFound},
		{syscall.ERROR_PATH_NOT_FOUND, spawnErrorNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.errno.Error(), func(t *testing.T) {
			assert.Equal(t, tt.want, classifySpawnError(pathErr(tt.errno)))
		})
	}
}

// TestClassifySpawnErrorBadExecutable starts a file that is not a valid executable, so the
// error is the one Windows actually returns rather than a constructed one.
func TestClassifySpawnErrorBadExecutable(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "agent-data-plane.exe")
	require.NoError(t, os.WriteFile(bin, []byte("not a binary"), 0600))

	err := exec.Command(bin).Start()
	require.Error(t, err)
	assert.Equal(t, spawnErrorExecFormat, classifySpawnError(err))
}
