// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestGrandchildInheritsParentOverriddenStdin asserts that when a builtin is
// invoked through RunCommandWithStdin (with an overridden stdin) and that
// builtin then dispatches a grandchild via RunCommand (the no-stdin variant),
// the grandchild's stdin is the same overridden reader rather than the
// top-level r.stdin. Regression test for the chain
//
//	xargs (reads stdin) → xargs -a empty file → wc -c
//
// where the inner xargs uses RunCommand because it reads from -a, not stdin.
// Without the closure-level stdin propagation in runner_exec.go, the
// grandchild wc reads bytes the outer xargs never tokenised, leaking content
// from the original pipe.
func TestGrandchildInheritsParentOverriddenStdin(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.txt"), nil, 0644))

	// Build > 64 KiB of stdin so xargs's bufio prefetch (readChunk = 64 KiB)
	// cannot drain the entire pipe before the first child runs. The single
	// "a" item sits at the front; the rest is leftover that, without the
	// fix, would be visible to the grandchild wc through r.stdin.
	stdin := bytes.NewReader([]byte("a\n" + strings.Repeat("x", 70*1024) + "\n"))
	var stdout, stderr bytes.Buffer

	parser := syntax.NewParser()
	prog, err := parser.Parse(strings.NewReader(
		"xargs -n1 -I{} xargs -a empty.txt wc -c"), "")
	require.NoError(t, err)

	runner, err := interp.New(
		interp.StdIO(stdin, &stdout, &stderr),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()
	runner.Dir = dir

	if err := runner.Run(context.Background(), prog); err != nil {
		var es interp.ExitStatus
		require.True(t, errors.As(err, &es), "unexpected error: %v", err)
		require.EqualValues(t, 0, es)
	}

	// Both grandchildren must see an isolated empty stdin (0 bytes), not
	// the leftover "x" payload still buffered in the outer pipe.
	out := stdout.String()
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		assert.Equal(t, "0", strings.TrimSpace(line),
			"grandchild wc reported non-zero bytes — stdin isolation broken (full output: %q)", out)
	}
}
