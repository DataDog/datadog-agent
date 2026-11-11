// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package awk

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestReadProgramStdinReadsBlockingOSPipe(t *testing.T) {
	var descriptors [2]int
	require.NoError(t, unix.Pipe(descriptors[:]))
	reader := os.NewFile(uintptr(descriptors[0]), "blocking-program-reader")
	writer := os.NewFile(uintptr(descriptors[1]), "blocking-program-writer")
	require.NotNil(t, reader)
	require.NotNil(t, writer)
	t.Cleanup(func() { _ = reader.Close() })
	require.Error(t, reader.SetReadDeadline(time.Time{}))

	const program = `BEGIN { print "ok" }`
	go func() {
		_, _ = writer.WriteString(program)
		_ = writer.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	total := 0

	text, err := readProgramCancellable(ctx, reader, &total)

	require.NoError(t, err)
	assert.Equal(t, program, text)
	assert.Equal(t, len(program), total)
}

func TestReadProgramStdinReadsBlockingNamedFIFO(t *testing.T) {
	path := filepath.Join(t.TempDir(), "program.fifo")
	require.NoError(t, syscall.Mkfifo(path, 0o600))
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NONBLOCK, 0)
	require.NoError(t, err)
	reader := os.NewFile(uintptr(fd), "blocking-program-fifo")
	require.NotNil(t, reader)
	t.Cleanup(func() { _ = reader.Close() })
	writer, err := os.OpenFile(path, os.O_WRONLY, 0)
	require.NoError(t, err)
	t.Cleanup(func() { _ = writer.Close() })
	require.NoError(t, unix.SetNonblock(fd, false))

	const program = `BEGIN { print "ok" }`
	_, err = writer.WriteString(program)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	total := 0

	text, err := readProgramCancellable(ctx, reader, &total)

	require.NoError(t, err)
	assert.Equal(t, program, text)
	assert.Equal(t, len(program), total)
}

func TestReadProgramStdinCancellationPreservesBlockingOSPipe(t *testing.T) {
	var descriptors [2]int
	require.NoError(t, unix.Pipe(descriptors[:]))
	reader := os.NewFile(uintptr(descriptors[0]), "blocking-program-reader")
	writer := os.NewFile(uintptr(descriptors[1]), "blocking-program-writer")
	require.NotNil(t, reader)
	require.NotNil(t, writer)
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	require.Error(t, reader.SetReadDeadline(time.Time{}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		total := 0
		_, err := readProgramCancellable(ctx, reader, &total)
		done <- err
	}()
	time.Sleep(2 * programReadWaitMilliseconds * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		_ = writer.Close()
		<-done
		t.Fatal("blocking program pipe read did not observe cancellation")
	}

	_, err := writer.WriteString("x")
	require.NoError(t, err)
	buf := make([]byte, 1)
	_, err = io.ReadFull(reader, buf)
	require.NoError(t, err)
	assert.Equal(t, "x", string(buf))
}

func TestReadProgramStdinPreservesExistingDeadline(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	t.Cleanup(func() { _ = writer.Close() })
	require.NoError(t, reader.SetReadDeadline(time.Now().Add(50*time.Millisecond)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	total := 0

	_, err = readProgramCancellable(ctx, reader, &total)

	require.ErrorIs(t, err, os.ErrDeadlineExceeded)
}

func TestReadProgramStdinClosedFileReturns(t *testing.T) {
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	require.NoError(t, writer.Close())

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	total := 0

	_, err = readProgramCancellable(ctx, reader, &total)

	require.Error(t, err)
	assert.NotErrorIs(t, err, context.DeadlineExceeded)
}
