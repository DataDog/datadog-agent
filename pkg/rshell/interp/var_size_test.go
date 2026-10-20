// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interp_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// TestOversizedInlineVarAbortsCommand verifies that an inline assignment whose
// value exceeds MaxVarBytes does NOT execute the following command and that the
// shell exits with a non-zero status.
func TestOversizedInlineVarAbortsCommand(t *testing.T) {
	large := strings.Repeat("x", interp.MaxVarBytes+1)
	script := fmt.Sprintf("X=%s echo SHOULD_NOT_RUN", large)

	stdout, stderr, code := runScript(t, script, "")

	assert.NotContains(t, stdout, "SHOULD_NOT_RUN", "command must not execute after oversized inline assignment")
	assert.Contains(t, stderr, "value too large")
	assert.NotEqual(t, 0, code, "exit code must be non-zero")
}

// TestTotalVarStorageCapEnforced verifies that creating many small variables
// eventually hits the MaxTotalVarsBytes limit and the shell exits with a
// non-zero status and an appropriate error message.
func TestTotalVarStorageCapEnforced(t *testing.T) {
	// Each variable holds 1024 bytes; 1025 variables would require > 1 MiB total.
	// Use a shell loop to assign them so that we exercise the interpreter's
	// variable-assignment path (not just Go-level API).
	//
	// We write the loop as a here-document-style multiline string passed to
	// runScript so that the script is parsed by the interpreter.
	//
	// The value is 1024 'x' characters; after ~1024 iterations the total
	// storage (1024 * 1024 = 1 MiB) should be exactly at the limit, and
	// the 1025th assignment should be rejected.
	value := strings.Repeat("x", 1024)
	// Build a script that assigns VAR_0 through VAR_1100 (enough to exceed 1 MiB).
	var sb strings.Builder
	for i := range 1101 {
		fmt.Fprintf(&sb, "VAR_%d=%s\n", i, value)
	}
	sb.WriteString("echo DONE\n")
	script := sb.String()

	stdout, stderr, code := runScript(t, script, "")

	assert.NotContains(t, stdout, "DONE", "echo must not run after total storage cap is exceeded")
	assert.Contains(t, stderr, "variable storage limit exceeded", "expected storage-cap error in stderr")
	assert.NotEqual(t, 0, code, "exit code must be non-zero after hitting total storage cap")
}

// TestSubshellTotalVarStorageDoubleCount is a regression test for a bug where
// overlayEnviron.Each emitted overridden variables twice — once from the parent
// environment and once from the overlay — causing newOverlayEnviron to seed
// totalBytes at 2× the real storage.  With the seed inflated past
// MaxTotalVarsBytes, any assignment inside the subshell was rejected even though
// actual memory use was within the cap.
func TestSubshellTotalVarStorageDoubleCount(t *testing.T) {
	// X is 600 KiB in the initial environment — well under the 1 MiB cap.
	value600K := strings.Repeat("x", 600*1024)

	// The script overrides X in the parent shell, putting it into the overlay.
	// Before the fix, Each() emitted X from both r.Env and the parent overlay,
	// seeding the subshell's totalBytes at 1200 KiB > MaxTotalVarsBytes.
	// Y=x inside the subshell was then rejected even though real storage is 600 KiB.
	script := fmt.Sprintf("X=%s\n( Y=x; echo SUBSHELL_OK )\necho DONE\n", value600K)

	stdout, _, code := runScript(t, script, "", interp.Env("X="+value600K))

	assert.Contains(t, stdout, "SUBSHELL_OK", "subshell assignment should succeed: real storage is 600 KiB, within the 1 MiB cap")
	assert.Contains(t, stdout, "DONE")
	assert.Equal(t, 0, code)
}

// TestNonBackgroundSubshellVarOverrideTracking is a regression test for the
// double-count bug in overlayEnviron.Set when a non-background subshell
// (created by ( ) or $( )) overrides a parent-inherited variable for the first
// time.  Before the fix, the parent variable's bytes were seeded into totalBytes
// by newOverlayEnviron AND charged again as a full "new" write (oldBytes=0),
// causing the delta to be inflated by len(prev.Str) and incorrectly hitting the
// cap even when real memory use was well within bounds.
func TestNonBackgroundSubshellVarOverrideTracking(t *testing.T) {
	// 512 KiB in parent; override with the same 512 KiB value inside a ( ) subshell.
	// Real subshell memory = 512 KiB — well within the 1 MiB cap.
	// Before the fix: totalBytes seed = 512 KiB, delta = +512 KiB (oldBytes=0),
	// sum = 1 MiB, then Y=z would push it over → cap hit.
	value512K := strings.Repeat("x", 512*1024)
	script := fmt.Sprintf("X=%s\n( X=%s; Y=z; echo OK )\necho DONE\n", value512K, value512K)

	stdout, _, code := runScript(t, script, "")

	assert.Contains(t, stdout, "OK", "subshell override should succeed: real storage is 512 KiB, within the 1 MiB cap")
	assert.Contains(t, stdout, "DONE")
	assert.Equal(t, 0, code)
}

// TestNonBackgroundSubshellDoesNotCountEnvVars is a regression test for a bug
// where newOverlayEnviron seeded totalBytes for non-background subshells by
// summing parent.Each(), which included variables provided via interp.Env().
// Because Env() variables are intentionally excluded from the cap in the top-level
// runner (via Reset()'s post-init totalBytes zero-out), counting them again in the
// subshell seed caused false cap violations: a legitimate caller passing a large
// Env() variable would find that any non-background subshell started with a
// pre-inflated counter, causing otherwise-valid assignments to be rejected.
func TestNonBackgroundSubshellDoesNotCountEnvVars(t *testing.T) {
	// Provide a 512 KiB variable via Env() — it must NOT count against the cap.
	// A non-background subshell that assigns another 600 KiB variable should
	// succeed, because the real script-assigned storage (600 KiB) is within the cap.
	envValue512K := strings.Repeat("x", 512*1024)
	script := fmt.Sprintf(
		"( A=%s; echo SUBSHELL_OK )\necho DONE\n",
		strings.Repeat("y", 600*1024),
	)

	stdout, _, code := runScript(t, script, "", interp.Env("CONFIG="+envValue512K))

	assert.Contains(t, stdout, "SUBSHELL_OK",
		"non-background subshell should not count Env() vars toward the cap")
	assert.Contains(t, stdout, "DONE")
	assert.Equal(t, 0, code)
}

// TestBackgroundSubshellCapEnforced verifies that a background (pipeline) subshell
// correctly inherits the parent's totalBytes counter and cannot allocate beyond
// MaxTotalVarsBytes. This exercises the background=true path in newOverlayEnviron.
func TestBackgroundSubshellCapEnforced(t *testing.T) {
	value900K := strings.Repeat("x", 900*1024)
	value200K := strings.Repeat("y", 200*1024)
	// Parent fills ~900 KiB. The pipeline right-hand side is a background subshell;
	// it tries to assign another 200 KiB which would push the total past 1 MiB.
	script := fmt.Sprintf("A=%s\necho test | { B=%s; echo SHOULD_NOT_PRINT; }\necho DONE\n",
		value900K, value200K)

	stdout, stderr, _ := runScript(t, script, "")

	assert.NotContains(t, stdout, "SHOULD_NOT_PRINT",
		"background subshell must not execute after total storage cap is exceeded")
	assert.Contains(t, stderr, "variable storage limit exceeded",
		"expected storage-cap error in stderr")
	assert.Contains(t, stdout, "DONE",
		"parent shell must continue after background subshell fails")
}

// TestTotalVarStorageCapUpdateTracking verifies that updating an existing variable
// correctly adjusts the total byte counter (i.e. growing a variable counts against
// the cap, and shrinking it frees space).
func TestTotalVarStorageCapUpdateTracking(t *testing.T) {
	// Assign a large value (512 KiB) twice to the SAME variable.
	// The total should stay at ~512 KiB (not 1 MiB) because we're updating, not creating.
	// Then reassign to empty, which should free the space so another 512 KiB variable fits.
	value512K := strings.Repeat("x", 512*1024)
	script := fmt.Sprintf(
		"A=%s\nA=%s\nA=\nB=%s\necho OK\n",
		value512K, value512K, value512K,
	)

	stdout, _, code := runScript(t, script, "")

	assert.Contains(t, stdout, "OK", "expected OK after update/shrink cycle")
	assert.Equal(t, 0, code, "exit code must be zero when total storage stays within cap")
}
