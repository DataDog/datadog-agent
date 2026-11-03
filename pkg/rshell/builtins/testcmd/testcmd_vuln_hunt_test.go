// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testcmd_test

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

// Vulnerability-hunt regression coverage for campaign 2026-05-19-codex.

func runTestcmdScriptErr(ctx context.Context, t *testing.T, script, dir string, opts ...interp.RunnerOption) (stdout, stderr string, exitCode int, runErr error) {
	t.Helper()

	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
	require.NoError(t, err)

	var outBuf, errBuf bytes.Buffer
	allOpts := append([]interp.RunnerOption{interp.StdIO(nil, &outBuf, &errBuf)}, opts...)
	runner, err := interp.New(allOpts...)
	require.NoError(t, err)
	t.Cleanup(func() { runner.Close() })
	if dir != "" {
		runner.Dir = dir
	}

	runErr = runner.Run(ctx, prog)
	if runErr != nil {
		var es interp.ExitStatus
		if errors.As(runErr, &es) {
			exitCode = int(es)
		} else {
			exitCode = -1
		}
	}
	return outBuf.String(), errBuf.String(), exitCode, runErr
}

func TestVulnHuntGTFOBinsKnown_TestcmdCannotCreateFilesWithRedirect(t *testing.T) {
	dir := t.TempDir()

	stdout, stderr, code := runScript(t,
		"test -n x > created.txt\n",
		dir,
		interp.AllowedPaths([]string{dir}),
	)
	assert.Equal(t, 2, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "> file redirection is not supported")
	_, err := os.Stat(filepath.Join(dir, "created.txt"))
	assert.True(t, errors.Is(err, os.ErrNotExist), "created.txt should not exist, stat err=%v", err)
}

func TestVulnHuntFileRead_TestcmdSymlinkEscapeDoesNotFollowOutsideAllowedPaths(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(allowed, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	writeFile(t, outside, "secret.txt", "classified\n")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(allowed, "link.txt")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	stdout, stderr, code := runScript(t,
		"test -f link.txt; echo f=$?\n"+
			"test -e link.txt; echo e=$?\n"+
			"test -h link.txt; echo h=$?\n",
		allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 0, code)
	assert.Equal(t, "f=1\ne=1\nh=0\n", stdout)
	assert.Empty(t, stderr)
}

func TestVulnHuntFileRead_TestcmdOutsideFileComparisonsDoNotLeakExistence(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(allowed, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	writeFile(t, allowed, "inside.txt", "inside\n")
	writeFile(t, outside, "outside.txt", "outside\n")

	existingOutside := filepath.Join(outside, "outside.txt")
	missingOutside := filepath.Join(outside, "missing.txt")
	script := "test inside.txt -nt " + existingOutside + "; echo existing_right=$?\n" +
		"test inside.txt -nt " + missingOutside + "; echo missing_right=$?\n" +
		"test " + existingOutside + " -nt inside.txt; echo existing_left=$?\n" +
		"test " + missingOutside + " -nt inside.txt; echo missing_left=$?\n" +
		"test " + existingOutside + " -ot inside.txt; echo existing_left_ot=$?\n" +
		"test " + missingOutside + " -ot inside.txt; echo missing_left_ot=$?\n"

	stdout, stderr, code := runScript(t, script, allowed, interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t,
		"existing_right=0\n"+
			"missing_right=0\n"+
			"existing_left=1\n"+
			"missing_left=1\n"+
			"existing_left_ot=0\n"+
			"missing_left_ot=0\n",
		stdout)
}

func TestVulnHuntFlagAbuse_TestcmdFlagLikeValuesRemainOperands(t *testing.T) {
	stdout, stderr, code := runScript(t,
		"test --no-such-flag; echo unknown=$?\n"+
			"test --; echo dashdash=$?\n"+
			`test "-f" = "-f"; echo flagstring=$?`+"\n",
		"",
	)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "unknown=0\ndashdash=0\nflagstring=0\n", stdout)
}

func TestVulnHuntStdinAbuse_TestcmdIgnoresPipeInput(t *testing.T) {
	stdout, stderr, code := runScript(t,
		`printf "classified\n" | test -n ok; echo pipe_true=$?`+"\n"+
			`printf "classified\n" | test; echo pipe_empty=$?`+"\n",
		"",
	)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "pipe_true=0\npipe_empty=1\n", stdout)
}

func TestVulnHuntParserConfusion_TestcmdBangDisambiguationEdges(t *testing.T) {
	stdout, stderr, code := runScript(t,
		"test a -a !; echo literal_bang=$?\n"+
			"test -n x -a !; echo missing_negand=$?\n"+
			"test ! = !; echo bang_compare=$?\n",
		"",
	)
	assert.Equal(t, 0, code)
	assert.Contains(t, stderr, "test: missing argument")
	assert.Equal(t, "literal_bang=0\nmissing_negand=2\nbang_compare=0\n", stdout)
}

func TestVulnHuntResourceExhaustion_TestcmdDeepNegationRejected(t *testing.T) {
	script := "test " + strings.Repeat("! ", 200) + `"x"`
	_, stderr, code := runScript(t, script, "")
	assert.Equal(t, 2, code)
	assert.Contains(t, stderr, "too deeply nested")
}

func TestVulnHuntSignalContext_PreCanceledTestcmdDoesNotDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stdout, stderr, _, err := runTestcmdScriptErr(ctx, t, "test -n SHOULD_NOT_RUN\n", "", interpoption.AllowAllCommands().(interp.RunnerOption))
	assert.True(t, errors.Is(err, context.Canceled), "expected context.Canceled, got %v", err)
	assert.Empty(t, stdout)
	assert.Empty(t, stderr)
}

func TestVulnHuntCompositionAttack_ExpandedOutsidePathStaysSandboxed(t *testing.T) {
	root := t.TempDir()
	allowed := filepath.Join(root, "allowed")
	outside := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(allowed, 0o755))
	require.NoError(t, os.MkdirAll(outside, 0o755))
	writeFile(t, outside, "secret.txt", "classified\n")

	stdout, stderr, code := runScript(t,
		"target="+filepath.ToSlash(filepath.Join(outside, "secret.txt"))+"\n"+
			`test -f "$target"; echo expanded=$?`+"\n"+
			`[ -f "$target" ]; echo bracket=$?`+"\n",
		allowed,
		interp.AllowedPaths([]string{allowed}),
	)
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Equal(t, "expanded=1\nbracket=1\n", stdout)
}
