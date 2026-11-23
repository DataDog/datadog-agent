// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || darwin

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var fifoInputScripts = []string{
	"cat < fifo",
	`echo "$(< fifo)"`,
	"cat < fifo; echo after",
	"{ cat; } < fifo",
	"x=$(< fifo)",
}

func runFIFOInputScript(ctx context.Context, t *testing.T, dir, script string) (stdout string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}), StdIO(nil, &out, &errOut))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	done := make(chan error, 1)
	go func() { done <- r.Run(ctx, parseScript(t, script)) }()
	select {
	case err = <-done:
		return out.String(), err
	case <-time.After(10 * time.Second):
		t.Fatalf("%q did not return after its context ended", script)
		return "", nil
	}
}

// TestInputRedirectFromFIFOWithoutWriterEndsWithContext verifies that a read
// from a FIFO that never gets a writer stops when the run's context ends.
func TestInputRedirectFromFIFOWithoutWriterEndsWithContext(t *testing.T) {
	for _, script := range fifoInputScripts {
		t.Run(script, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, unix.Mkfifo(filepath.Join(dir, "fifo"), 0o600))

			ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
			defer cancel()
			stdout, err := runFIFOInputScript(ctx, t, dir, script)
			assert.ErrorIs(t, err, context.DeadlineExceeded)
			assert.Empty(t, stdout)
		})
	}
}

// TestInputRedirectFromFIFOWithoutWriterEndsWithMaxExecutionTime verifies the
// same for the runner's own execution-time bound.
func TestInputRedirectFromFIFOWithoutWriterEndsWithMaxExecutionTime(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, "fifo"), 0o600))

	var out, errOut bytes.Buffer
	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}), StdIO(nil, &out, &errOut), MaxExecutionTime(200*time.Millisecond))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	done := make(chan error, 1)
	go func() { done <- r.Run(context.Background(), parseScript(t, "cat < fifo")) }()
	select {
	case err = <-done:
		assert.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(10 * time.Second):
		t.Fatal("cat < fifo did not return after MaxExecutionTime")
	}
}

// TestInputRedirectFromFIFOReadsWriterData verifies that data written to a
// FIFO still reaches the reader through both input forms.
func TestInputRedirectFromFIFOReadsWriterData(t *testing.T) {
	for _, script := range []string{"cat < fifo", `echo "$(< fifo)"`} {
		t.Run(script, func(t *testing.T) {
			dir := t.TempDir()
			fifo := filepath.Join(dir, "fifo")
			require.NoError(t, unix.Mkfifo(fifo, 0o600))

			writeErr := make(chan error, 1)
			go func() {
				w, err := os.OpenFile(fifo, os.O_WRONLY, 0)
				if err != nil {
					writeErr <- err
					return
				}
				_, err = w.WriteString("hello\n")
				if cerr := w.Close(); err == nil {
					err = cerr
				}
				writeErr <- err
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			stdout, err := runFIFOInputScript(ctx, t, dir, script)
			require.NoError(t, err)
			require.NoError(t, <-writeErr)
			assert.Equal(t, "hello\n", stdout)
		})
	}
}

// TestInputRedirectFromRegularFileKeepsFileStdin verifies that a regular-file
// input redirect still hands the command the file itself rather than a pipe.
func TestInputRedirectFromRegularFileKeepsFileStdin(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"), []byte("data\n"), 0o600))

	var out, errOut bytes.Buffer
	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}), StdIO(nil, &out, &errOut))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir
	r.Reset()

	cls, err := r.redir(context.Background(), parseScript(t, "cat < in.txt").Stmts[0].Redirs[0])
	require.NoError(t, err)
	defer cls.Close()
	require.NotNil(t, r.stdin)
	info, err := r.stdin.Stat()
	require.NoError(t, err)
	assert.True(t, info.Mode().IsRegular())
}

// TestInputRedirectFromFIFOAttachesStdinPipe verifies that a FIFO input
// redirect hands the command a pipe and that its closer owns that pipe.
func TestInputRedirectFromFIFOAttachesStdinPipe(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, unix.Mkfifo(filepath.Join(dir, "fifo"), 0o600))

	var out, errOut bytes.Buffer
	r, err := New(allowAllCommandsOpt(), AllowedPaths([]string{dir}), StdIO(nil, &out, &errOut))
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir
	r.Reset()

	cls, err := r.redir(context.Background(), parseScript(t, "true < fifo").Stmts[0].Redirs[0])
	require.NoError(t, err)
	defer cls.Close()
	ic, ok := cls.(inputRedirCloser)
	require.True(t, ok, "closer is %T", cls)
	require.NotNil(t, r.stdin)
	assert.Same(t, r.stdin, ic.pipe)
}

// endlessReader always returns data, so a copy goroutine feeding a pipe from it
// can only stop once a write to the pipe fails.
type endlessReader struct{}

func (endlessReader) Read(p []byte) (int, error) { return len(p), nil }
func (endlessReader) Close() error               { return nil }

// TestInputRedirectCloserReleasesStdinPipe verifies that closing an input
// redirect releases the stdin pipe, so the copy goroutine exits even when the
// command never drained it and the source never ends.
func TestInputRedirectCloserReleasesStdinPipe(t *testing.T) {
	pr, done, err := stdinPipe(context.Background(), endlessReader{})
	require.NoError(t, err)

	require.NoError(t, inputRedirCloser{src: endlessReader{}, pipe: pr}.Close())
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("stdin copy goroutine did not exit")
	}
}
