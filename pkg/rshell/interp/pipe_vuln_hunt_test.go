// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// vuln-hunt campaign: 2026-05-19-codex
// Target: pipe (shell-feature)

package interp

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVulnHuntShellFeatureReadonlyBypass_PipelineStagesPreserveReadonly(t *testing.T) {
	stdout, stderr := runScriptWithReadonly(t,
		"{ RO_VAR=hacked; echo left=$RO_VAR; } | cat\n"+
			"echo seed | { RO_VAR=hacked; echo right=$RO_VAR; }\n"+
			"echo after=$RO_VAR\n")

	assert.Contains(t, stderr, "readonly variable",
		"pipeline stage assignment to readonly must produce readonly errors")
	assert.NotContains(t, stdout, "hacked",
		"pipeline stages must not observe or leak a bypassed readonly value")
	assert.Contains(t, stdout, "after=original",
		"parent must retain readonly value after both pipeline stages")
}

func TestVulnHuntShellFeatureSignalContext_PipeWriterStopsAfterReaderExit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows broken-pipe error format differs from unix; tested on linux/macos only")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "big.txt"),
		[]byte(strings.Repeat("line\n", 256*1024)),
		0644,
	))

	var stdout, stderr bytes.Buffer
	r, err := New(
		allowAllCommandsOpt(),
		AllowedPaths([]string{dir}),
		StdIO(nil, &stdout, &stderr),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })
	r.Dir = dir

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = r.Run(ctx, parseScript(t, "cat big.txt | head -n 1\n"))
	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "pipeline did not finish before context deadline")
	assert.Equal(t, "line\n", stdout.String())
	assert.Empty(t, stderr.String())
}
