// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func runLogicOpsVulnHunt(t *testing.T, ctx context.Context, script, dir string, stdin io.Reader, configure func(*Runner)) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(stdin, &stdout, &stderr), allowAllCommandsOpt(), AllowedPaths([]string{dir}))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Dir = dir
	r.Reset()
	if configure != nil {
		configure(r)
	}

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	runErr := r.Run(ctx, prog)
	return stdout.String(), stderr.String(), runErr
}

func setReadonlyVulnHuntVar(t *testing.T, r *Runner) {
	t.Helper()
	require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
		Set:      true,
		Kind:     expand.String,
		Str:      "original",
		ReadOnly: true,
	}))
}

func TestVulnHuntShellFeatureRedirectionChain_LogicSkippedOutputRedirectDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "created.txt")

	stdout, stderr, err := runLogicOpsVulnHunt(t, context.Background(), "false && echo hidden > created.txt\n", dir, nil, nil)

	assert.Equal(t, "", stdout)
	assert.Equal(t, "> file redirection is not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
	_, statErr := os.Stat(created)
	assert.True(t, os.IsNotExist(statErr), "skipped output redirect must not create files")
}

func TestVulnHuntShellFeatureReadonlyBypass_LogicSkippedBranchDoesNotMutateReadonly(t *testing.T) {
	dir := t.TempDir()
	var runner *Runner

	stdout, stderr, err := runLogicOpsVulnHunt(t, context.Background(), "false && RO_VAR=hacked echo hidden\n", dir, nil, func(r *Runner) {
		runner = r
		setReadonlyVulnHuntVar(t, r)
	})

	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
	assert.Equal(t, ExitStatus(1), err)
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly)
}

func TestVulnHuntShellFeatureReadonlyBypass_LogicExecutedBranchPreservesReadonly(t *testing.T) {
	dir := t.TempDir()
	var runner *Runner

	stdout, stderr, err := runLogicOpsVulnHunt(t, context.Background(), "RO_VAR=hacked true && echo hidden\n", dir, nil, func(r *Runner) {
		runner = r
		setReadonlyVulnHuntVar(t, r)
	})

	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "readonly variable")
	assert.Equal(t, ExitStatus(1), err)
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly)
}

func TestVulnHuntShellFeatureSignalContext_LogicSkippedBlockingStdinDoesNotStart(t *testing.T) {
	dir := t.TempDir()
	pr, pw, err := os.Pipe()
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = pr.Close()
		_ = pw.Close()
	})

	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(pr, &stdout, &stderr), allowAllCommandsOpt(), AllowedPaths([]string{dir}))
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Dir = dir
	r.Reset()

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader("false && cat\n"), "")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		done <- r.Run(context.Background(), prog)
	}()

	select {
	case err := <-done:
		assert.Equal(t, ExitStatus(1), err)
		assert.Equal(t, "", stdout.String())
		assert.Equal(t, "", stderr.String())
	case <-time.After(700 * time.Millisecond):
		_ = pw.Close()
		t.Fatal("skipped cat branch started and blocked on stdin")
	}
}

func TestVulnHuntShellFeatureSignalContext_LogicCanceledLongChainStops(t *testing.T) {
	dir := t.TempDir()
	script := strings.Repeat("true && ", 1000) + "echo hidden\n"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, _, err := runLogicOpsVulnHunt(t, ctx, script, dir, nil, nil)

	assert.Equal(t, "", stdout)
	assert.True(t, errors.Is(err, context.Canceled), "canceled context should stop before executing the chain, got %v", err)
}
