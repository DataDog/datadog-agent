// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package free_test

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

func runScript(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, "")
}

// --- Help / usage ---

func TestFreeHelp(t *testing.T) {
	stdout, stderr, code := runScript(t, "free --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: free")
	assert.Contains(t, stdout, "--human")
	assert.Contains(t, stdout, "--help")
}

func TestFreeHelp_ShortCircuitsBeforeOtherErrors(t *testing.T) {
	// GNU-style: --help short-circuits even with other junk present.
	stdout, stderr, code := runScript(t, "free --help --bogus")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage: free")
}

// --- Errors ---

func TestFreeExtraOperand(t *testing.T) {
	stdout, stderr, code := runScript(t, "free foo")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "free: extra operand 'foo'\nTry 'free --help' for more information.\n", stderr)
}

func TestFreeUnknownFlag(t *testing.T) {
	_, stderr, code := runScript(t, "free --bogus")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "free:")
}

func TestFreeHelpRejectsExplicitValue(t *testing.T) {
	// GNU getopt semantics: no-argument flags reject --flag=value, even
	// --flag=true. Matches df's noArgBool behaviour.
	_, stderr, code := runScript(t, "free --help=true")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "doesn't allow an argument")
}

func TestFreeHumanRejectsExplicitValue(t *testing.T) {
	_, stderr, code := runScript(t, "free --human=true")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "doesn't allow an argument")
}

// --- Platform behavior ---

func TestFreeNotSupportedOffLinux(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("free is supported on linux; see TestFreeLinuxHappyPath")
	}
	stdout, stderr, code := runScript(t, "free")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Equal(t, "free: not supported on this platform\n", stderr)
}

// TestFreeLinuxHappyPath exercises the real /proc/meminfo of the machine
// running the test. It only checks structural output (headers, row
// labels, exit code) since the actual memory numbers are host-dependent
// and non-deterministic — exact-value coverage lives in
// builtins/internal/meminfo's parseMeminfo tests, which run against
// fixed fixtures instead of live kernel state.
func TestFreeLinuxHappyPath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("free is only supported on linux")
	}
	stdout, stderr, code := runScript(t, "free")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "total")
	assert.Contains(t, stdout, "used")
	assert.Contains(t, stdout, "free")
	assert.Contains(t, stdout, "shared")
	assert.Contains(t, stdout, "buff/cache")
	assert.Contains(t, stdout, "available")
	assert.Contains(t, stdout, "Mem:")
	assert.Contains(t, stdout, "Swap:")
}

func TestFreeLinuxHumanFlag(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("free is only supported on linux")
	}
	stdout, stderr, code := runScript(t, "free -h")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Mem:")
	assert.Contains(t, stdout, "Swap:")
}

// TestFreeDoesNotTouchAllowedPaths verifies free works with an empty
// AllowedPaths sandbox — the /proc/meminfo read is documented as exempt.
func TestFreeDoesNotTouchAllowedPaths(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("free is only supported on linux")
	}
	stdout, stderr, code := testutil.RunScript(t, "free", "", interp.AllowedPaths(nil))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Mem:")
}
