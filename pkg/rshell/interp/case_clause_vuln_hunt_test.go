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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

func runCaseClauseVulnHunt(t *testing.T, ctx context.Context, script, dir string, configure func(*Runner)) (string, string, error) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	r, err := New(StdIO(nil, &stdout, &stderr), allowAllCommandsOpt(), AllowedPaths([]string{dir}))
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

func TestVulnHuntShellFeatureRedirectionChain_CaseOutputRedirectDoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()
	created := filepath.Join(dir, "created.txt")

	stdout, stderr, err := runCaseClauseVulnHunt(t, context.Background(), "case x in x) echo hidden;; esac > created.txt\n", dir, nil)

	assert.Equal(t, "", stdout)
	assert.Equal(t, "case statements are not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
	_, statErr := os.Stat(created)
	assert.True(t, os.IsNotExist(statErr), "case output redirect must not create files before validation rejects the case node")
}

func TestVulnHuntShellFeatureReadonlyBypass_CaseArmDoesNotMutateReadonly(t *testing.T) {
	dir := t.TempDir()
	var runner *Runner

	stdout, stderr, err := runCaseClauseVulnHunt(t, context.Background(), "case x in x) RO_VAR=hacked echo \"$RO_VAR\";; esac\n", dir, func(r *Runner) {
		runner = r
		require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
			Set:      true,
			Kind:     expand.String,
			Str:      "original",
			ReadOnly: true,
		}))
	})

	assert.Equal(t, "", stdout)
	assert.Equal(t, "case statements are not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly, "case arm execution must not clear readonly state")
}

func TestVulnHuntShellFeatureReadonlyBypass_CaseSubjectCmdSubstDoesNotMutateReadonly(t *testing.T) {
	dir := t.TempDir()
	var runner *Runner

	stdout, stderr, err := runCaseClauseVulnHunt(t, context.Background(), "case \"$(RO_VAR=hacked echo subject)\" in subject) echo hidden;; esac\n", dir, func(r *Runner) {
		runner = r
		require.NoError(t, r.writeEnv.Set("RO_VAR", expand.Variable{
			Set:      true,
			Kind:     expand.String,
			Str:      "original",
			ReadOnly: true,
		}))
	})

	assert.Equal(t, "", stdout)
	assert.Equal(t, "case statements are not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
	vr := runner.lookupVar("RO_VAR")
	assert.Equal(t, "original", vr.Str)
	assert.True(t, vr.ReadOnly, "case subject expansion must not clear readonly state")
}

func TestVulnHuntShellFeatureSignalContext_CanceledContextDoesNotEnterCaseExecution(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, stderr, err := runCaseClauseVulnHunt(t, ctx, "case x in x) echo hidden;; esac\n", dir, nil)

	assert.Less(t, time.Since(start), 2*time.Second, "case validation should return promptly with a canceled context")
	assert.Equal(t, "case statements are not supported\n", stderr)
	assert.Equal(t, ExitStatus(2), err)
}
