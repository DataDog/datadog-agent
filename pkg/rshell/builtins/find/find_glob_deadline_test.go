// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package find_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// findGlobDeadlineSetup builds a 64 KiB "./" run in the shell, so a small
// script yields a long start path and a long -path pattern whose literal tail
// nearly matches at every offset before failing.
const findGlobDeadlineSetup = `p=./; for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15; do p=$p$p; done; `

// runFindWithDeadline runs script with a short execution deadline and fails
// the test if the run does not return well within a fixed bound. The run
// happens in a goroutine so a regression fails promptly instead of hanging
// the test binary until the go test timeout.
func runFindWithDeadline(t *testing.T, script, dir string) (stdout string, runErr error) {
	t.Helper()
	const deadline = 500 * time.Millisecond
	const bound = 5 * time.Second

	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	var outBuf, errBuf bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &outBuf, &errBuf),
		interp.AllowedPaths([]string{dir}),
		interpoption.AllowAllCommands().(interp.RunnerOption),
	)
	require.NoError(t, err)
	t.Cleanup(func() { runner.Close() })
	runner.Dir = dir

	done := make(chan error, 1)
	start := time.Now()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		done <- runner.Run(ctx, prog)
	}()
	select {
	case err := <-done:
		t.Logf("run returned after %v", time.Since(start))
		return outBuf.String(), err
	case <-time.After(bound):
		t.Fatalf("find did not honour a %v deadline: still running after %v", deadline, bound)
		return "", nil
	}
}

func TestFindPathGlobHonoursDeadline(t *testing.T) {
	for _, pred := range []string{"-path", "-ipath"} {
		t.Run(pred, func(t *testing.T) {
			dir := t.TempDir()
			_, err := runFindWithDeadline(t,
				findGlobDeadlineSetup+`find $p$p$p$p. -maxdepth 0 `+pred+` "*$p${p}X"`, dir)
			assert.Error(t, err)
		})
	}
}

// A negated match must not print the entry when the matcher was cut short:
// the partial result is not a real "no match".
func TestFindNegatedPathGlobCancelledPrintsNothing(t *testing.T) {
	dir := t.TempDir()
	stdout, err := runFindWithDeadline(t,
		findGlobDeadlineSetup+`find $p$p$p$p. -maxdepth 0 ! -path "*$p${p}X"`, dir)
	assert.Error(t, err)
	assert.Empty(t, stdout)
}

// A long bracket class is scanned in full on every backtrack step, so the
// class scan itself must observe cancellation: an ~8 KiB start path and a
// 1 MiB class that never matches would otherwise scan gigabytes between the
// matcher's outer checks.
func TestFindPathGlobBracketClassHonoursDeadline(t *testing.T) {
	for _, pred := range []string{"-path", "-ipath"} {
		t.Run(pred, func(t *testing.T) {
			dir := t.TempDir()
			_, err := runFindWithDeadline(t,
				`p=./; for i in 1 2 3 4 5 6 7 8 9 10 11 12; do p=$p$p; done; `+
					`b=a; for i in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19; do b=$b$b; done; `+
					`find "$p." -maxdepth 0 `+pred+` "*[$b$b]"`, dir)
			assert.Error(t, err)
		})
	}
}
