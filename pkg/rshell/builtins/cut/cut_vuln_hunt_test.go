// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Blocked-attack regression tests added by the vuln-hunt campaign
// 2026-05-18-initial-audit (target: cut).
package cut_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/internal/interpoption"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// H2: a symlink inside AllowedPaths that points outside the sandbox must not
// reveal the target's content. allowedpaths uses os.Root which refuses to
// follow such symlinks.
func TestVulnHuntBuiltinFileAccessBypass_SymlinkOutsideSandbox(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks are restricted on Windows")
	}
	allowed := t.TempDir()
	parent := filepath.Dir(allowed)
	require.NoError(t, os.WriteFile(filepath.Join(parent, "secret-out.txt"),
		[]byte("S3CR3T\n"), 0644))
	t.Cleanup(func() { _ = os.Remove(filepath.Join(parent, "secret-out.txt")) })
	require.NoError(t, os.Symlink(filepath.Join(parent, "secret-out.txt"),
		filepath.Join(allowed, "link")))

	stdout, stderr, code := testutil.RunScript(t, "cut -c1- link", allowed,
		interp.AllowedPaths([]string{allowed}))
	assert.NotEqual(t, 0, code, "symlink-out attack must fail")
	assert.NotContains(t, stdout, "S3CR3T")
	assert.NotEmpty(t, stderr)
}

// H3: --output-delimiter accepts arbitrary strings. Newlines / NUL bytes /
// ANSI escapes inside the delimiter pass through to stdout verbatim, but this
// stays inside the global stdout cap and therefore inside the sandbox.
// Regression: confirm the value is forwarded as-is (so callers can't smuggle
// metacharacters back into a re-parsing shell — but the rshell child never
// re-parses cut output, so the boundary is observational only).
func TestVulnHuntBuiltinFlagDrivenExploit_OutputDelimiterControlBytes(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "in.txt"),
		[]byte("a:b:c\n"), 0644))
	// Embed a newline in the delimiter via $'\n'. The interpreter should
	// honour the ANSI-C quoting, and the bytes should appear in stdout
	// between fields.
	stdout, _, code := testutil.RunScript(t,
		"cut -d: --output-delimiter=$'\\n' -f1,3 in.txt", dir,
		interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "a\nc\n", stdout)
}

// H5: a line larger than MaxLineBytes (1 MiB) is rejected by the line scanner
// with a non-zero exit and an explanatory stderr message.
func TestVulnHuntBuiltinResourceExhaustion_LineExceedsMaxLineBytes(t *testing.T) {
	dir := t.TempDir()
	body := make([]byte, (1<<20)+128) // > 1 MiB, no newline yet
	for i := range body {
		body[i] = 'a'
	}
	body = append(body, '\n')
	require.NoError(t, os.WriteFile(filepath.Join(dir, "huge.txt"), body, 0644))

	_, stderr, code := testutil.RunScript(t, "cut -c1-5 huge.txt > /dev/null", dir,
		interp.AllowedPaths([]string{dir}))
	assert.NotEqual(t, 0, code, "over-cap line must produce an error")
	assert.True(t,
		strings.Contains(stderr, "cut:") || strings.Contains(stderr, "too long"),
		"stderr should report the cap, got %q", stderr)
}

// H7/H12: many disjoint ranges plus a large output delimiter can amplify
// output from a small input, but the interpreter-level stdout cap must stop it.
func TestVulnHuntBuiltinResourceExhaustion_OutputLimitStopsDelimiterAmplification(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input.txt"),
		[]byte(strings.Repeat("a", 1200)+"\n"), 0644))

	rangeParts := make([]string, 0, 600)
	for i := 1; i <= 1200; i += 2 {
		rangeParts = append(rangeParts, strconv.Itoa(i))
	}
	delimiter := strings.Repeat("x", 20000)
	script := "cut -b" + strings.Join(rangeParts, ",") + " --output-delimiter='" + delimiter + "' input.txt"

	prog, err := syntax.NewParser().Parse(strings.NewReader(script), "")
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

// H8: -b range with a value that overflows strconv.Atoi must be rejected.
func TestVulnHuntBuiltinIntegerOverflow_RangeAtoiOverflow(t *testing.T) {
	dir := t.TempDir()
	cases := []string{
		"99999999999999999999",
		"99999999999999999999999",
		"-b99999999999999999999",
	}
	for _, val := range cases {
		t.Run(val, func(t *testing.T) {
			_, stderr, code := testutil.RunScript(t,
				"echo abc | cut -b"+val, dir, interp.AllowedPaths([]string{dir}))
			assert.Equal(t, 1, code, "overflow value must exit 1")
			assert.Contains(t, stderr, "cut:")
		})
	}
}

// H9: a very large unbounded end like -b1-99999999999 stores end as an int
// (int64 on 64-bit). The processing loop is bounded by line length, so no
// overflow path is reachable. Confirm the command succeeds and emits the
// expected line content (echo's output, no truncation).
func TestVulnHuntBuiltinIntegerOverflow_LargeUnboundedEnd(t *testing.T) {
	dir := t.TempDir()
	stdout, _, code := testutil.RunScript(t,
		"echo hello | cut -b1-99999999999", dir, interp.AllowedPaths([]string{dir}))
	assert.Equal(t, 0, code)
	assert.Equal(t, "hello\n", stdout)
}
