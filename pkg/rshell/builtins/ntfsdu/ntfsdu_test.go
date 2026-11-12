// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// The entire ntfsdu package is Windows-only (//go:build windows) and the
// command is registered only on Windows (see interp/register_builtins_windows.go),
// so all of its tests — these flag-parsing / help / error tests plus the
// parseSize and scan tests — run on Windows only. The sole non-Windows test is
// ntfsdu_other_test.go, which asserts the command is absent off Windows.

package ntfsdu_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// cmdRun runs a script with the temp dir as the sole AllowedPaths root. The
// behavior asserted here (flag parsing, --help, argument validation) is
// platform-independent, but ntfs-du is only registered on Windows (see the file
// header), so these tests run there.
func cmdRun(t *testing.T, script string) (string, string, int) {
	t.Helper()
	dir := t.TempDir()
	return testutil.RunScript(t, script, dir, interp.AllowedPaths([]string{dir}))
}

func TestHelpToStdout(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ntfs-du --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	for _, want := range []string{
		"Usage: ntfs-du",
		"find large folders, files, and file extensions",
		"--max-depth",
		"--tree-limit",
		"--find-ext",
		"--output",
	} {
		assert.Contains(t, stdout, want)
	}
}

func TestUnknownFlagRejected(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ntfs-du --bogus")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "unrecognized option '--bogus'")
	assert.Contains(t, stderr, "Try 'ntfs-du --help'")
}

func TestInvalidMinRejected(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ntfs-du --min 10Q C:\\")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "invalid --min value")
}

func TestNegativeMaxDepthRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ntfs-du --max-depth -1 C:\\")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid --max-depth")
}

func TestFindLimitTooLargeRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ntfs-du --find-limit 5000 C:\\")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "exceeds maximum")
}

func TestFindLimitZeroWithFindQueryRejected(t *testing.T) {
	// --find-limit 0 alongside a --find query would silently discard the matches
	// the caller asked for, so it must be rejected rather than emit an empty block.
	_, stderr, code := cmdRun(t, "ntfs-du --find-ext .dmp --find-limit 0 C:\\")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "--find-limit 0 would discard")
}

func TestFindLimitZeroWithoutFindQueryAllowed(t *testing.T) {
	// --find-limit 0 with no --find query is a harmless no-op: the limit never
	// applies, so it must not trip the contradiction guard. --help short-circuits
	// before the scan, keeping this test off the Windows-only raw-volume path.
	_, stderr, code := cmdRun(t, "ntfs-du --find-limit 0 --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
}

func TestTopFilesTooLargeRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ntfs-du --top-files 5000 C:\\")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "exceeds maximum")
}

func TestTopExtensionsAndTreeLimitTooLargeRejected(t *testing.T) {
	for _, script := range []string{
		"ntfs-du --top-ext 65 C:\\",
		"ntfs-du --tree-limit 1001 C:\\",
	} {
		_, stderr, code := cmdRun(t, script)
		assert.Equal(t, 1, code, "script: %s", script)
		assert.Contains(t, stderr, "exceeds maximum", "script: %s", script)
	}
}

func TestInvalidOutputRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ntfs-du --output yaml C:\\")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid --output format")
}

func TestTooManyOperandsRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ntfs-du a b")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "at most one folder operand")
}

func TestInvalidValueBeforeHelpErrors(t *testing.T) {
	// An invalid flag value ahead of --help must report the value, not print
	// help (matches head/tail; see the validation-ordering comment in ntfsdu.go).
	stdout, stderr, code := cmdRun(t, "ntfs-du --max-depth -1 --help")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid --max-depth")
	assert.NotContains(t, stdout, "Usage: ntfs-du", "help must not print when a flag value is invalid")
}

func TestOperandCountLosesToHelp(t *testing.T) {
	// Positional-operand errors, by contrast, yield to --help (GNU semantics).
	stdout, stderr, code := cmdRun(t, "ntfs-du a b --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: ntfs-du")
}

func TestJSONBadDoesNotLeakToStdout(t *testing.T) {
	// A validation failure must not print a partial JSON document.
	stdout, _, code := cmdRun(t, "ntfs-du --top-files -5 C:\\")
	assert.Equal(t, 1, code)
	assert.False(t, strings.Contains(stdout, "{"), "no JSON should be emitted on error")
}
