// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ss_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// runScript runs a shell script and returns stdout, stderr, and the exit code.
func runScript(t *testing.T, script, dir string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, dir, opts...)
}

// cmdRun runs an ss command with no AllowedPaths restriction.
// ss reads from kernel interfaces (proc/net on Linux, sysctls on macOS), not
// from files in AllowedPaths, so no path restriction is applied here.
func cmdRun(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return runScript(t, script, "")
}

// --- Help flag ---

func TestSSHelp(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ss -h")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: ss")
	assert.Empty(t, stderr)
}

func TestSSHelpLong(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ss --help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: ss")
	assert.Empty(t, stderr)
}

// --- Unknown / rejected flags ---

func TestSSUnknownFlag(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss --no-such-flag")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// TestSSRejectedFlagF verifies that -F (--filter, GTFOBins file-read vector)
// is not accepted and causes ss to exit with code 1.
func TestSSRejectedFlagF(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss -F /dev/null")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// TestSSRejectedFlagP verifies that -p (--processes, PID disclosure) is
// rejected with exit code 1.
func TestSSRejectedFlagP(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss -p")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// TestSSRejectedFlagK verifies that -K (--kill, writes to kernel) is
// rejected with exit code 1.
func TestSSRejectedFlagK(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss -K")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// TestSSRejectedFlagE verifies that -E (--events, infinite stream) is
// rejected with exit code 1.
func TestSSRejectedFlagE(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss -E")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// TestSSRejectedFlagN verifies that -N (--net, namespace switching) is
// rejected with exit code 1.
func TestSSRejectedFlagN(t *testing.T) {
	_, stderr, code := cmdRun(t, "ss -N ns0")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ss:")
}

// --- Valid flags that must not produce a parse error ---
// On non-Linux platforms the platform stub returns exit 1 with "not supported",
// so we only assert that the error is NOT a flag-parse error (i.e. stderr does
// not contain "unknown flag").

func TestSSNumericFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -n")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSNoHeaderFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -H")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSSummaryFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -s")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSCombinedFlagsAccepted(t *testing.T) {
	// -tan = TCP + All + Numeric: all three flags are valid.
	_, stderr, _ := cmdRun(t, "ss -tan")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSListeningFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -l")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSIPv4FlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -4")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}

func TestSSIPv6FlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ss -6")
	assert.NotContains(t, stderr, "unknown flag")
	assert.NotContains(t, stderr, "unknown shorthand")
}
