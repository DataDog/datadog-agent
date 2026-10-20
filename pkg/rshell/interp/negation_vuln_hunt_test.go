// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"
)

func runNegationVulnHunt(t *testing.T, ctx context.Context, script, dir string, opts ...RunnerOption) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	allOpts := []RunnerOption{StdIO(nil, &stdout, &stderr), allowAllCommandsOpt(), AllowedPaths([]string{dir})}
	allOpts = append(allOpts, opts...)
	r, err := New(allOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { r.Close() })
	r.Dir = dir

	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)
	runErr := r.Run(ctx, prog)
	return stdout.String(), stderr.String(), runErr
}

func TestVulnHuntShellFeatureParserConfusion_NegationDoesNotInvertValidationError(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "created.txt")

	stdout, stderr, err := runNegationVulnHunt(t, context.Background(), "! echo hidden > created.txt\necho after\n", dir)

	assert.Equal(t, "", stdout)
	assert.Equal(t, "> file redirection is not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
	_, statErr := os.Stat(created)
	assert.True(t, errors.Is(statErr, os.ErrNotExist), "blocked negated redirect created %s", created)
}

func TestVulnHuntShellFeatureSignalContext_NegationDoesNotInvertTimeout(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, err := runNegationVulnHunt(t, context.Background(), "! while true; do true; done\n", dir, MaxExecutionTime(20*time.Millisecond))

	assert.Equal(t, "", stdout)
	assert.Equal(t, "", stderr)
	assert.True(t, errors.Is(err, context.DeadlineExceeded), "negation must not mask timeout, got %v", err)
}
