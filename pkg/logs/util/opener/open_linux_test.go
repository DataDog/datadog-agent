// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package opener

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/logs/types"
)

func TestOpenLogFileWithDirect(t *testing.T) {
	content := make([]byte, directIOAlignment*2+211)
	for i := range content {
		content[i] = byte(i % 251)
	}
	path := filepath.Join(t.TempDir(), "direct.log")
	require.NoError(t, os.WriteFile(path, content, 0600))

	file, err := NewFileOpener().OpenLogFileWithFlags(path, []types.FileOpenFlag{types.FileOpenFlagDirect})
	if errors.Is(err, unix.EINVAL) || errors.Is(err, unix.EOPNOTSUPP) {
		t.Skipf("test filesystem does not support O_DIRECT: %v", err)
	}
	require.NoError(t, err)
	t.Cleanup(func() { _ = file.Close() })

	_, err = file.Seek(13, io.SeekStart)
	require.NoError(t, err)
	got := make([]byte, 2048)
	_, err = io.ReadFull(file, got)
	require.NoError(t, err)
	require.Equal(t, content[13:13+len(got)], got)
}

func TestOpenLogFileWithDirectReportsPermissionErrorAsUnsupported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses DAC permission checks, so O_DIRECT open would not hit EACCES")
	}
	path := filepath.Join(t.TempDir(), "noperm.log")
	require.NoError(t, os.WriteFile(path, []byte("data"), 0o000))

	_, err := NewFileOpener().OpenLogFileWithFlags(path, []types.FileOpenFlag{types.FileOpenFlagDirect})
	// A file we can't open directly must surface as unsupported so the caller
	// falls back to a buffered (possibly privileged) open and memoizes it.
	require.ErrorIs(t, err, ErrOpenFlagsUnsupported)
}
