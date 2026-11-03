// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package logrotate_test

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// A FIFO with no reader attached must fail fast. The sandbox opens write
// targets with O_NONBLOCK, so the O_WRONLY open returns ENXIO immediately
// instead of blocking forever waiting for a reader.
func TestLogrotateFifoWithoutReaderDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "app.log")
	require.NoError(t, mkfifo(fifoPath))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, stderr, code := runScriptCtx(ctx, t, "logrotate --force app.log", dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)

	require.NoError(t, ctx.Err(), "logrotate on a readerless FIFO must not block")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, `logrotate: "app.log":`)

	info, err := os.Lstat(fifoPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe, "the FIFO must survive untouched")
}

// With a reader attached the O_WRONLY open succeeds, so the non-regular-file
// guard on the resulting fd is the thing that has to reject it. Without that
// guard logrotate would ftruncate a pipe.
func TestLogrotateFifoWithReaderRejectedAsNonRegular(t *testing.T) {
	dir := t.TempDir()
	fifoPath := filepath.Join(dir, "app.log")
	require.NoError(t, mkfifo(fifoPath))

	// O_RDONLY|O_NONBLOCK returns immediately on a FIFO and holds the read
	// end open for the duration of the test.
	reader, err := os.OpenFile(fifoPath, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	require.NoError(t, err)
	defer reader.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stdout, stderr, code := runScriptCtx(ctx, t, "logrotate --force app.log", dir,
		interp.AllowedPaths([]string{dir + ":rw"}),
		interp.WithMode(interp.ModeRemediation),
	)

	require.NoError(t, ctx.Err(), "logrotate on a FIFO must not block")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "not a regular file")

	info, err := os.Lstat(fifoPath)
	require.NoError(t, err)
	assert.NotZero(t, info.Mode()&os.ModeNamedPipe, "the FIFO must survive untouched")
}
