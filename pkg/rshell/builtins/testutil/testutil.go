// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package testutil provides shared test helpers for builtin command tests.
package testutil

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// repeatReader is an io.Reader that repeats a fixed line pattern indefinitely.
type repeatReader struct {
	line []byte
	pos  int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if r.pos >= len(r.line) {
			r.pos = 0
		}
		copied := copy(p[n:], r.line[r.pos:])
		r.pos += copied
		n += copied
	}
	return n, nil
}

// NewRepeatReader returns an io.Reader that yields the given line pattern
// indefinitely. Use io.LimitReader to cap the total bytes produced.
// It is intended for benchmark setup — generating large synthetic files
// without keeping the full content in memory.
func NewRepeatReader(line string) io.Reader {
	return &repeatReader{line: []byte(line)}
}

// RunScriptCtx runs a shell script with a context and returns stdout, stderr,
// and the exit code. It accepts testing.TB so it can be used in both tests
// and benchmarks.
func RunScriptCtx(ctx context.Context, t testing.TB, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption)}, opts...)
	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(ctx, prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// FuzzIterDir creates an isolated per-iteration subdirectory under baseDir,
// using counter to generate a unique name. It returns the directory path and
// a cleanup function that removes the directory. This replaces the ~12-line
// boilerplate pattern (atomic counter + MkdirAll + defer RemoveAll) that was
// previously duplicated across 30+ fuzz functions.
func FuzzIterDir(t testing.TB, baseDir string, counter *atomic.Int64) (string, func()) {
	t.Helper()
	n := counter.Add(1)
	dir := filepath.Join(baseDir, fmt.Sprintf("iter%d", n))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	return dir, func() {
		if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
			t.Errorf("cleanup %s: %v", dir, err)
		}
	}
}

// FuzzRunScriptCtx runs a script in fuzz mode with AllowedPaths set to [dir].
// This ensures fuzz iterations can actually read/write files inside their
// iteration directory rather than silently failing with permission errors.
func FuzzRunScriptCtx(ctx context.Context, t testing.TB, script, dir string) (string, string, int) {
	t.Helper()
	return RunScriptCtx(ctx, t, script, dir, interp.AllowedPaths([]string{dir}))
}

// RunScript runs a shell script and returns stdout, stderr, and the exit code.
// It accepts testing.TB so it can be used in both tests and benchmarks.
func RunScript(t testing.TB, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return RunScriptCtx(context.Background(), t, script, dir, opts...)
}

// RunScriptDiscard runs a shell script and returns stderr and the exit code.
// Stdout is discarded (io.Discard). Use this in memory-allocation tests to
// prevent output buffering from dominating the AllocedBytesPerOp measurement.
func RunScriptDiscard(t testing.TB, script, dir string, opts ...interp.RunnerOption) (string, int) {
	t.Helper()
	return RunScriptDiscardCtx(context.Background(), t, script, dir, opts...)
}

// RunScriptDiscardCtx is RunScriptDiscard with an explicit context.
func RunScriptDiscardCtx(ctx context.Context, t testing.TB, script, dir string, opts ...interp.RunnerOption) (string, int) {
	t.Helper()
	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, io.Discard, &errBuf), interpoption.AllowAllCommands().(interp.RunnerOption)}, opts...)
	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	defer runner.Close()

	if dir != "" {
		runner.Dir = dir
	}

	err = runner.Run(ctx, prog)
	exitCode := 0
	if err != nil {
		var es interp.ExitStatus
		if errors.As(err, &es) {
			exitCode = int(es)
		} else if ctx.Err() == nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	return errBuf.String(), exitCode
}
