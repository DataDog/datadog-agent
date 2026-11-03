// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package df_test

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

// dfRun runs a df-only script with no AllowedPaths. df does not touch
// the sandbox, so we don't need any path access.
func dfRun(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return runScript(t, script, "")
}

// requireSupported skips the test if df returns "not supported" — i.e.
// we are running on Windows / a platform without a backend.
func requireSupported(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skipf("df is not supported on %s", runtime.GOOS)
	}
}

// --- Help / usage ---

func TestDfHelp(t *testing.T) {
	stdout, stderr, code := dfRun(t, "df --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: df")
	assert.Contains(t, stdout, "--human-readable")
	assert.Contains(t, stdout, "--portability")
	assert.Contains(t, stdout, "--inodes")
	// -k has no long form (matches GNU df). The help line must
	// render as a plain "-k", not the bogus "-k, --" pflag would
	// emit for an empty long name.
	assert.Contains(t, stdout, "-k ")
	assert.NotContains(t, stdout, "-k, --")
	assert.NotContains(t, stdout, "--kibibytes")
	// No-argument flags use a NUL-byte NoOptDefVal sentinel so
	// `--all=true` etc. are rejected. pflag.PrintDefaults would
	// otherwise render that NUL into the help text — make sure the
	// printHelp wrapper clears NoOptDefVal first so the help stream
	// stays plain ASCII.
	assert.NotContains(t, stdout, "\x00")
	assert.NotContains(t, stdout, "[= ]")
}

// --- Default output structure ---

func TestDfDefaultColumns(t *testing.T) {
	requireSupported(t)
	stdout, stderr, code := dfRun(t, "df")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.NotEmpty(t, lines)
	header := lines[0]
	for _, want := range []string{"Filesystem", "1K-blocks", "Used", "Available", "Use%", "Mounted on"} {
		assert.Contains(t, header, want, "header %q missing %q", header, want)
	}
}

func TestDfHumanReadable(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df -h")
	assert.Equal(t, 0, code)
	header := firstLine(stdout)
	assert.Contains(t, header, "Size")
	assert.NotContains(t, header, "1K-blocks")
}

func TestDfSI(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df -H")
	assert.Equal(t, 0, code)
	header := firstLine(stdout)
	assert.Contains(t, header, "Size")
}

func TestDfPosix(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df -P")
	assert.Equal(t, 0, code)
	// POSIX format: -P changes the column labels but keeps GNU's
	// aligned column layout. Verify the header words appear in
	// order and "Capacity" replaces "Use%".
	header := firstLine(stdout)
	wantOrder := []string{"Filesystem", "1024-blocks", "Used", "Available", "Capacity", "Mounted on"}
	prev := -1
	for _, w := range wantOrder {
		idx := strings.Index(header, w)
		assert.GreaterOrEqual(t, idx, 0, "%q missing from header %q", w, header)
		assert.Greater(t, idx, prev, "%q out of order in header %q", w, header)
		prev = idx
	}
}

func TestDfPrintType(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df -T")
	assert.Equal(t, 0, code)
	header := firstLine(stdout)
	assert.Contains(t, header, "Type")
	// Type column is between Filesystem and 1K-blocks.
	fIdx := strings.Index(header, "Filesystem")
	tIdx := strings.Index(header, "Type")
	bIdx := strings.Index(header, "1K-blocks")
	assert.True(t, fIdx < tIdx && tIdx < bIdx, "header %q has Type out of place", header)
}

func TestDfInodes(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df -i")
	assert.Equal(t, 0, code)
	header := firstLine(stdout)
	assert.Contains(t, header, "Inodes")
	assert.Contains(t, header, "IUsed")
	assert.Contains(t, header, "IFree")
}

func TestDfTotal(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df --total")
	assert.Equal(t, 0, code)
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	last := lines[len(lines)-1]
	assert.True(t, strings.HasPrefix(last, "total"), "last line should start with 'total': %q", last)
}

func TestDfAll(t *testing.T) {
	requireSupported(t)
	stdoutAll, _, codeAll := dfRun(t, "df -a")
	stdoutDefault, _, codeDefault := dfRun(t, "df")
	assert.Equal(t, 0, codeAll)
	assert.Equal(t, 0, codeDefault)
	// On most hosts, -a returns at least as many rows as the default
	// listing. (On a Linux container where /proc has only the root
	// mount they can be equal, hence >=.)
	allLines := lineCount(stdoutAll)
	defLines := lineCount(stdoutDefault)
	assert.GreaterOrEqual(t, allLines, defLines)
}

func TestDfTypeFilter_NoMatches(t *testing.T) {
	requireSupported(t)
	// GNU df: when -t leaves no rows, exit 1 with a stderr message.
	// Scripts use this exit status to test filesystem presence.
	_, stderr, code := dfRun(t, "df -t no-such-fs-type")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "no file systems processed")
}

func TestDfNoSyncIsNoop(t *testing.T) {
	requireSupported(t)
	a, _, _ := dfRun(t, "df")
	b, _, _ := dfRun(t, "df --no-sync")
	// Both should at least produce the same header.
	assert.Equal(t, firstLine(a), firstLine(b))
}

// --- Error paths ---

func TestDfRejectedSyncFlag(t *testing.T) {
	stdout, stderr, code := dfRun(t, "df --sync")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "df:")
	assert.Contains(t, stderr, "--sync")
}

func TestDfUnknownFlag(t *testing.T) {
	_, stderr, code := dfRun(t, "df --no-such-flag")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "df:")
}

func TestDfBlockSizeNotSupported(t *testing.T) {
	// -B / --block-size is intentionally not implemented in v1.
	_, stderr, code := dfRun(t, "df -B 1M")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "df:")
}

func TestDfOutputNotSupported(t *testing.T) {
	// --output is intentionally not implemented in v1.
	_, stderr, code := dfRun(t, "df --output=source,fstype")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "df:")
}

func TestDfExtraOperand(t *testing.T) {
	// File operands are intentionally not supported in v1.
	stdout, stderr, code := dfRun(t, "df /tmp")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "df:")
	assert.Contains(t, stderr, "extra operand")
}

// TestDfExtraOperandEscapesControlBytes verifies that a crafted operand
// containing a newline and an ANSI escape sequence is neutralized before
// being written to stderr, rather than letting it forge additional
// diagnostic lines or inject terminal control sequences.
func TestDfExtraOperandEscapesControlBytes(t *testing.T) {
	script := "df \"evil\nrshell: forged line\x1b[31m\"\n"
	_, stderr, code := dfRun(t, script)
	assert.Equal(t, 1, code)
	assert.NotContains(t, stderr, "evil\nrshell: forged line")
	assert.NotContains(t, stderr, "\x1b[31m")
	assert.Contains(t, stderr, `evil\nrshell: forged line\x1b[31m`)
}

func TestDfMultipleExtraOperands(t *testing.T) {
	_, stderr, code := dfRun(t, "df /tmp /var")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "df:")
}

func TestDfHelpExitCode(t *testing.T) {
	_, _, code := dfRun(t, "df --help")
	assert.Equal(t, 0, code)
}

// --- Integration with shell features ---

func TestDfPipeToWc(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "df | wc -l")
	assert.Equal(t, 0, code)
	// At least 2 lines: header + at least one mount.
	got := strings.TrimSpace(stdout)
	assert.NotEqual(t, "0", got)
	assert.NotEqual(t, "1", got)
}

func TestDfInForLoop(t *testing.T) {
	requireSupported(t)
	stdout, _, code := dfRun(t, "for i in 1 2; do df --help | head -n 1; done")
	assert.Equal(t, 0, code)
	// Help line printed twice.
	assert.Equal(t, 2, strings.Count(stdout, "Usage: df"))
}

// --- Context cancellation ---
//
// End-to-end cancellation through the runner is timing-sensitive: a
// pre-cancelled context aborts the runner before df ever executes, so the
// helper returns exit code 0 with no output. The cancellation contract
// inside df is exercised by the diskstats parser tests, which feed an
// already-cancelled context directly to parseMountInfo and assert it
// returns context.Canceled. End-to-end coverage is unnecessary.

// --- helpers ---

func firstLine(s string) string {
	before, _, _ := strings.Cut(s, "\n")
	return before
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimRight(s, "\n"), "\n") + 1
}
