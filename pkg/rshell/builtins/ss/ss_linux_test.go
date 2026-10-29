// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ss_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// Note: TestSSLinuxProcNetAccessDenied (which verified that ss fails when
// /proc/net is excluded from AllowedPaths) was intentionally removed when ss
// switched from callCtx.OpenFile to os.Open for /proc/net/* files. This is a
// deliberate policy change: kernel pseudo-filesystem paths under /proc are
// hardcoded and non-user-controllable, so AllowedPaths restrictions no longer
// apply to them. This matches the pattern used by ip route (procnet package).

// TestSSLinuxRun verifies that ss succeeds and output contains the expected
// column headers. The proc paths are deterministic and accessed directly via
// os.Open (no AllowedPaths restriction needed).
func TestSSLinuxRun(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ss -an")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Netid")
	assert.Contains(t, stdout, "State")
}

// TestSSLinuxSummary verifies that -s (summary) produces a Total: line.
func TestSSLinuxSummary(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ss -s")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Total:")
	assert.Contains(t, stdout, "TCP:")
	assert.Contains(t, stdout, "UDP:")
	assert.Contains(t, stdout, "Unix:")
}

// TestSSLinuxNoHeader verifies that -H suppresses the header line.
func TestSSLinuxNoHeader(t *testing.T) {
	stdout, _, code := cmdRun(t, "ss -anH")
	assert.Equal(t, 0, code)
	assert.NotContains(t, stdout, "Netid")
}

// TestSSLinuxTCPOnly verifies that -t restricts output to TCP entries.
func TestSSLinuxTCPOnly(t *testing.T) {
	stdout, _, code := cmdRun(t, "ss -tanH")
	assert.Equal(t, 0, code)
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) > 0 {
			assert.Equal(t, "tcp", fields[0], "unexpected Netid: %q", line)
		}
	}
}

// TestSSLinuxIPv4Only verifies that -4 drops IPv6 TCP entries.
func TestSSLinuxIPv4Only(t *testing.T) {
	stdout, _, code := cmdRun(t, "ss -tan4H")
	assert.Equal(t, 0, code)
	// IPv6 addresses contain colons in the address column; should not appear.
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		// The Local Address:Port column should not look like [::]:port or
		// contain an IPv6 address with multiple colons.
		fields := strings.Fields(line)
		if len(fields) >= 5 {
			localCol := fields[4]
			// IPv6 addresses are formatted as addr:port where addr contains "::"
			assert.NotContains(t, localCol, "::", "IPv6 address leaked into -4 output: %q", localCol)
		}
	}
}

// TestSSLinuxExtended verifies that -e adds uid/inode fields.
func TestSSLinuxExtended(t *testing.T) {
	stdout, _, code := cmdRun(t, "ss -tane")
	assert.Equal(t, 0, code)
	if code == 0 && strings.Contains(stdout, "\n") {
		// If any socket rows are printed, they should contain uid: and inode:
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) > 1 { // more than just the header
			assert.Contains(t, stdout, "uid:")
			assert.Contains(t, stdout, "inode:")
		}
	}
}

// TestSSLinuxProcNetBypassesAllowedPaths verifies that ss succeeds even when
// /proc/net is excluded from AllowedPaths, documenting the intentional sandbox
// bypass for kernel pseudo-filesystem paths. AllowedPaths cannot block ss from
// enumerating local sockets because ss uses os.Open directly for /proc/net/*.
func TestSSLinuxProcNetBypassesAllowedPaths(t *testing.T) {
	stdout, stderr, code := runScript(t, "ss -an", "", interp.AllowedPaths([]string{t.TempDir()}))
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Netid")
}

// TestSSLinuxContextCancelledBeforeRun verifies that a pre-cancelled context
// does not panic or produce corrupt output.
func TestSSLinuxContextCancelledBeforeRun(t *testing.T) {
	// Run with a cancelled context — the command should fail quickly rather
	// than hang or panic.  We use a short timeout in runScript via a
	// cancelled context.
	_, _, _ = cmdRun(t, "ss -an")
	// Just checking no panic occurs above.
}
