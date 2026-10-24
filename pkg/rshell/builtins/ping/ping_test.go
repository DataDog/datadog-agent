// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ping_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
	"github.com/DataDog/datadog-agent/pkg/rshell/interp"
)

// runScript runs a shell script and returns stdout, stderr, and the exit code.
func runScript(t *testing.T, script string, opts ...interp.RunnerOption) (string, string, int) {
	t.Helper()
	return testutil.RunScript(t, script, "", opts...)
}

// runScriptCtx runs a shell script with the given context.
func runScriptCtx(ctx context.Context, t *testing.T, script string) (string, string, int) {
	t.Helper()
	return testutil.RunScriptCtx(ctx, t, script, "")
}

// cmdRun runs a ping command with no path restrictions (ping uses the network,
// not the AllowedPaths sandbox).
func cmdRun(t *testing.T, script string) (string, string, int) {
	t.Helper()
	return runScript(t, script)
}

// skipIfNoNet skips the test unless RSHELL_NET_TEST=1 is set.
func skipIfNoNet(t *testing.T) {
	t.Helper()
	if os.Getenv("RSHELL_NET_TEST") == "" {
		t.Skip("skipping network test (set RSHELL_NET_TEST=1 to enable)")
	}
}

// ============================================================================
// Help flag
// ============================================================================

func TestPingHelp(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ping --help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: ping")
	assert.Contains(t, stdout, "-c")
	assert.Contains(t, stdout, "-W")
	assert.Contains(t, stdout, "-i")
	assert.Contains(t, stdout, "-q")
	assert.Contains(t, stdout, "-4")
	assert.Contains(t, stdout, "-6")
	assert.Empty(t, stderr)
}

func TestPingHelpShort(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ping -h")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "Usage: ping")
	assert.Empty(t, stderr)
}

func TestPingHelpMentionsFloodBlocked(t *testing.T) {
	stdout, _, code := cmdRun(t, "ping --help")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "-f")
}

// ============================================================================
// Argument validation
// ============================================================================

func TestPingMissingHost(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ping")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "ping: missing host operand")
}

func TestPingTooManyArgs(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ping host1 host2")
	assert.Equal(t, 1, code)
	assert.Empty(t, stdout)
	assert.Contains(t, stderr, "ping: too many arguments")
}

// ============================================================================
// Unknown / blocked flags
// ============================================================================

func TestPingUnknownFlag(t *testing.T) {
	_, stderr, code := cmdRun(t, "ping --no-such-flag localhost")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ping:")
}

func TestPingFloodFlagRejected(t *testing.T) {
	// -f (flood) is a DoS vector; must be rejected.
	_, stderr, code := cmdRun(t, "ping -f localhost")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ping:")
}

func TestPingBroadcastFlagRejected(t *testing.T) {
	// -b (broadcast) is not implemented.
	_, stderr, code := cmdRun(t, "ping -b 255.255.255.255")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ping:")
}

func TestPingSizeFlagRejected(t *testing.T) {
	// -s (packet size) is not implemented.
	_, stderr, code := cmdRun(t, "ping -s 1000 localhost")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ping:")
}

func TestPingInterfaceFlagRejected(t *testing.T) {
	// -I (interface) is not implemented.
	_, stderr, code := cmdRun(t, "ping -I eth0 localhost")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ping:")
}

// ============================================================================
// Flag acceptance — these tests verify flags are parsed without crashing.
// They use a non-existent host so the actual ping fails fast with exit 1,
// but the important thing is the flags are accepted (not rejected as unknown).
// ============================================================================

func TestPingCountFlagAccepted(t *testing.T) {
	// -c is a registered flag; should not give "unknown flag" error.
	_, stderr, _ := cmdRun(t, "ping -c 2 no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingWaitFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ping -W 500ms no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingIntervalFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ping -i 500ms no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingQuietFlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ping -q no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingIPv4FlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ping -4 no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingIPv6FlagAccepted(t *testing.T) {
	_, stderr, _ := cmdRun(t, "ping -6 no-such-host.invalid")
	assert.NotContains(t, stderr, "unknown flag")
}

// ============================================================================
// Context cancellation — RunWithContext must respect context deadline.
// ============================================================================

func TestPingContextCancel(t *testing.T) {
	// Very short deadline: the ping should return promptly, not hang.
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	start := time.Now()
	// Use a real-looking but unreachable address to ensure pro-bing actually
	// tries to send packets (so context cancellation is exercised).
	_, _, _ = runScriptCtx(ctx, t, "ping -c 10 -W 5s -i 1s 192.0.2.1")
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 3*time.Second, "ping should have been cancelled within deadline")
}

// ============================================================================
// Network tests (require RSHELL_NET_TEST=1)
// ============================================================================

func TestPingLocalhost(t *testing.T) {
	skipIfNoNet(t)
	stdout, stderr, code := cmdRun(t, "ping -c 2 127.0.0.1")
	assert.Equal(t, 0, code, "localhost ping should succeed; stderr: %s", stderr)
	assert.Contains(t, stdout, "PING")
	assert.Contains(t, stdout, "ping statistics")
	assert.Contains(t, stdout, "packets transmitted")
}

func TestPingLocalhostIPv4Flag(t *testing.T) {
	skipIfNoNet(t)
	stdout, _, code := cmdRun(t, "ping -4 -c 2 127.0.0.1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "PING")
}

func TestPingIPv6Localhost(t *testing.T) {
	skipIfNoNet(t)
	// Use -6 with the IPv6 loopback address. This covers the ipv6 branch in buildPinger.
	// Skip if IPv6 is not available on this system.
	stdout, stderr, code := cmdRun(t, "ping -6 -c 2 ::1")
	if code != 0 {
		// IPv6 may not be available in all CI environments; skip rather than fail.
		t.Skipf("IPv6 ping to ::1 failed (code=%d, stderr=%s); IPv6 may not be available", code, stderr)
	}
	assert.Contains(t, stdout, "PING")
}

func TestPingQuietOutput(t *testing.T) {
	skipIfNoNet(t)
	stdout, _, code := cmdRun(t, "ping -q -c 2 127.0.0.1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "ping statistics")
	// In quiet mode, no per-packet lines starting with "bytes from"
	assert.NotContains(t, stdout, "bytes from")
}

func TestPingCountClamp(t *testing.T) {
	skipIfNoNet(t)
	// -c 0 is clamped to 1; should send exactly 1 packet, not hang.
	start := time.Now()
	stdout, _, code := cmdRun(t, "ping -c 0 127.0.0.1")
	elapsed := time.Since(start)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "1 packets transmitted")
	assert.Less(t, elapsed, 10*time.Second, "clamped -c 0 should complete quickly")
}

func TestPingCountLargeClamp(t *testing.T) {
	skipIfNoNet(t)
	// -c 9999 is clamped to 20; command should finish, not hang for hours.
	start := time.Now()
	stdout, _, code := cmdRun(t, "ping -c 9999 -i 50ms -W 500ms 127.0.0.1")
	elapsed := time.Since(start)
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "20 packets transmitted")
	// 20 packets at 50ms interval with 500ms wait each = ~20 * 550ms = 11s max.
	assert.Less(t, elapsed, 30*time.Second)
}

func TestPingIntervalClamp(t *testing.T) {
	skipIfNoNet(t)
	// -i 10ms is below the 200ms minimum floor; should still work (clamped).
	_, stderr, code := cmdRun(t, "ping -c 2 -i 10ms 127.0.0.1")
	assert.Equal(t, 0, code)
	assert.NotContains(t, stderr, "unknown flag")
}

func TestPingStatisticsOutputFormat(t *testing.T) {
	skipIfNoNet(t)
	stdout, _, code := cmdRun(t, "ping -c 2 127.0.0.1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "packets transmitted")
	assert.Contains(t, stdout, "received")
	assert.Contains(t, stdout, "packet loss")
	// Statistics include RTT when packets were received.
	assert.Contains(t, stdout, "round-trip min/avg/max/stddev")
}

func TestPingUnreachableHostExitCode(t *testing.T) {
	skipIfNoNet(t)
	// An unresolvable host should exit 1.
	_, _, code := cmdRun(t, "ping -c 1 -W 1s no-such-host-xyzzy-invalid.example")
	assert.Equal(t, 1, code)
}
