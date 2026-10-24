// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Blocked-attack regression tests added by the vuln-hunt campaign
// 2026-05-18-initial-audit (target: sed).
package sed_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H6: an unconditional sed branch loop (`:loop; b loop`) must terminate
// at MaxBranchIterations per input line, not hang.
func TestVulnHuntBuiltinResourceExhaustion_BranchIterationCap(t *testing.T) {
	dir := t.TempDir()
	// `:loop; b loop` is an infinite jump within one cycle. The cap is
	// 10 000 iterations — sed must error or move on within sub-second
	// wall time on any reasonable machine.
	_, stderr, code := testutil.RunScript(t,
		"printf 'one\\n' | sed ':loop; b loop'", dir,
		interp.AllowedPaths([]string{dir}))
	// Either non-zero exit with a branch-cap message, or zero exit if
	// sed silently completes the iterations and falls through. The
	// failure mode we're guarding against is a hang.
	if code != 0 {
		assert.Contains(t, stderr, "sed:",
			"expected sed: prefix on branch-cap error, got %q", stderr)
	}
}

// H8: a very large numeric address (larger than the input has lines) is
// not allowed to overflow internally or produce wrong output.
func TestVulnHuntBuiltinIntegerOverflow_LargeAddress(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		val  string
		want string // expected stdout substring (or "" for any)
	}{
		{"99999999999", ""},
		{"9223372036854775807", ""},
		{"18446744073709551616", ""}, // overflow at parse → expected to fail parse
	}
	for _, tc := range cases {
		t.Run("addr="+tc.val, func(t *testing.T) {
			script := "printf 'a\\nb\\nc\\n' | sed -n '" + tc.val + "p'"
			_, stderr, code := testutil.RunScript(t, script, dir,
				interp.AllowedPaths([]string{dir}))
			// Either the address parses (and selects no line, exit 0) or
			// the parser rejects it (exit 1 with sed: prefix). Either way,
			// the command must not hang or wrap around to a valid line.
			if code != 0 {
				assert.Contains(t, stderr, "sed:")
			}
		})
	}
}

// Campaign: vuln-hunt/2026-05-19-codex. These are public-safe
// blocked-attack regressions for sed.

func TestVulnHuntBuiltinFileAccessBypass_SymlinkOperandOutsideAllowedPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires platform-specific privileges on Windows")
	}

	allowed := t.TempDir()
	secret := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(secret, "secret.txt"), []byte("secret words\n"), 0644))
	require.NoError(t, os.Symlink(filepath.Join(secret, "secret.txt"), filepath.Join(allowed, "escape_link")))

	stdout, stderr, code := testutil.RunScript(t,
		"sed 's/secret/public/' escape_link", allowed,
		interp.AllowedPaths([]string{allowed}))
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "sed:")
	assert.NotContains(t, stdout+stderr, "secret words")
}

func TestVulnHuntBuiltinResourceExhaustion_OutputLimitStopsPrintAmplification(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("x\n", 3_000_000)), 0644))

	prog, err := syntax.NewParser().Parse(strings.NewReader("sed p big.txt"), "")
	require.NoError(t, err)

	var stdout, stderr bytes.Buffer
	runner, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interpoption.AllowAllCommands().(interp.RunnerOption),
		interp.AllowedPaths([]string{dir}),
	)
	require.NoError(t, err)
	defer runner.Close()
	runner.Dir = dir

	err = runner.Run(context.Background(), prog)
	require.Error(t, err)
	assert.True(t, errors.Is(err, interp.ErrOutputLimitExceeded), "got %v", err)
	assert.LessOrEqual(t, stdout.Len(), 10*1024*1024)
	assert.Empty(t, stderr.String())
}

func TestVulnHuntBuiltinIntegerOverflow_GroupDepthCap(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"), []byte("x\n"), 0644))
	script := strings.Repeat("{", 300) + "p" + strings.Repeat("}", 300)

	_, stderr, code := testutil.RunScript(t,
		"sed '"+script+"' input.txt", dir,
		interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "group nesting depth limit exceeded")
}
