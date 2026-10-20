// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func runGlobbingVulnHunt(t *testing.T, script, dir string, configure func(*Runner), opts ...RunnerOption) (string, string, error, *Runner) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	allOpts := append([]RunnerOption{StdIO(nil, &stdout, &stderr), allowAllCommandsOpt()}, opts...)
	r, err := New(allOpts...)
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

	runErr := r.Run(context.Background(), prog)
	return stdout.String(), stderr.String(), runErr, r
}

func TestVulnHuntShellFeatureRedirectionChain_GlobDefaultNoAllowedPathsBlocked(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("secret\n"), 0644))

	stdout, stderr, err, _ := runGlobbingVulnHunt(t, "echo *\n", dir, nil)

	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "permission denied")
	var es ExitStatus
	assert.ErrorAs(t, err, &es)
}

func TestVulnHuntShellFeatureReadonlyBypass_ForLoopGlobDoesNotMutateReadonly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644))

	stdout, stderr, err, runner := runGlobbingVulnHunt(t, "for RO_VAR in *.txt; do echo \"$RO_VAR\"; done\n", dir, func(r *Runner) {
		require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
			Set:      true,
			Kind:     expand.String,
			Str:      "original",
			ReadOnly: true,
		}))
	}, AllowedPaths([]string{dir}))

	require.NoError(t, err)
	assert.Equal(t, "original\n", stdout)
	assert.Contains(t, stderr, "readonly variable")
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly)
}

func TestVulnHuntShellFeatureReadonlyBypass_InlineAssignmentGlobDoesNotMutateReadonly(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644))

	stdout, stderr, err, runner := runGlobbingVulnHunt(t, "RO_VAR=*.txt echo hidden\n", dir, func(r *Runner) {
		require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
			Set:      true,
			Kind:     expand.String,
			Str:      "original",
			ReadOnly: true,
		}))
	}, AllowedPaths([]string{dir}))

	assert.Equal(t, "", stdout)
	assert.Contains(t, stderr, "readonly variable")
	var es ExitStatus
	assert.ErrorAs(t, err, &es)
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly)
}
