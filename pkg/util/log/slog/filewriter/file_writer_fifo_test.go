// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !aix

package filewriter

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRollingFileWriterKeepsFIFOMode(t *testing.T) {
	setUmask(t)
	tempDir := t.TempDir()
	setLogDir(t, tempDir)
	logPath := filepath.Join(tempDir, "test.log")
	require.NoError(t, syscall.Mkfifo(logPath, 0644))

	// Open the read end first, so that the writer opens the write end
	// without blocking.
	r, err := os.OpenFile(logPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	require.NoError(t, err)
	defer r.Close()

	writer, err := NewRollingFileWriterSize(logPath, 1024, 5, RollingNameModePostfix)
	require.NoError(t, err)
	defer writer.Close()

	_, err = writer.Write([]byte("test\n"))
	require.NoError(t, err)

	info, err := os.Stat(logPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0644), info.Mode().Perm())
}
