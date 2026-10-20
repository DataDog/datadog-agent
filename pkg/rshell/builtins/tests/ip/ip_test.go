// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ip_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// ip addr show — success cases
// ============================================================================

// TestIPAddrShowExitsZero verifies "ip addr show" runs successfully.
func TestIPAddrShowExitsZero(t *testing.T) {
	stdout, _, code := cmdRun(t, "ip addr show")
	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout)
}

// TestIPAddrShowContainsInetLine verifies "ip addr show" includes at least one
// "inet " line (a v4 or v6 address).
func TestIPAddrShowContainsInetLine(t *testing.T) {
	stdout, _, code := cmdRun(t, "ip addr show")
	assert.Equal(t, 0, code)
	assert.True(t, strings.Contains(stdout, "inet ") || strings.Contains(stdout, "inet6 "),
		"expected inet or inet6 in output, got: %q", stdout)
}

// TestIPAddrShowDevLoopback verifies filtering by loopback interface name.
func TestIPAddrShowDevLoopback(t *testing.T) {
	lo := loopbackName(t)
	stdout, stderr, code := cmdRun(t, fmt.Sprintf(`ip addr show dev "%s"`, lo))
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, lo)
	assert.Contains(t, stdout, "127.0.0.1")
}

// TestIPAddrShowIPv4Only verifies -4 restricts output to IPv4 (inet lines).
func TestIPAddrShowIPv4Only(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip -4 addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "inet 127.0.0.1")
}

// TestIPAddrShowIPv6Only verifies -6 restricts output to IPv6 lines.
func TestIPAddrShowIPv6Only(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip -6 addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	// May have inet6 ::1 or nothing if IPv6 is not configured, but no inet4 lines.
	assert.NotContains(t, stdout, "    inet ")
}

// TestIPAddrShowBothFiltersCancel verifies -4 -6 together cancel both filters.
func TestIPAddrShowBothFiltersCancel(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip -4 -6 addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	// Both filters cancelled: all families shown.
	assert.Contains(t, stdout, "inet ")
}

// TestIPAddrShowDefaultCmdOmitted verifies "ip addr" (no "show") works the same.
func TestIPAddrShowDefaultCmdOmitted(t *testing.T) {
	lo := loopbackName(t)
	stdout1, _, code1 := cmdRun(t, fmt.Sprintf(`ip addr show dev "%s"`, lo))
	stdout2, _, code2 := cmdRun(t, fmt.Sprintf(`ip addr dev "%s"`, lo))
	assert.Equal(t, 0, code1)
	assert.Equal(t, 0, code2)
	assert.Equal(t, stdout1, stdout2)
}

// TestIPAddrObjectAliases verifies "address" is an alias for "addr".
func TestIPAddrObjectAliases(t *testing.T) {
	lo := loopbackName(t)
	stdout1, _, _ := cmdRun(t, fmt.Sprintf(`ip addr show dev "%s"`, lo))
	stdout2, _, _ := cmdRun(t, fmt.Sprintf(`ip address show dev "%s"`, lo))
	assert.Equal(t, stdout1, stdout2)
}

// TestIPAddrListAliases verifies "list" and "lst" are aliases for "show".
func TestIPAddrListAliases(t *testing.T) {
	lo := loopbackName(t)
	base, _, _ := cmdRun(t, fmt.Sprintf(`ip addr show dev "%s"`, lo))
	list, _, _ := cmdRun(t, fmt.Sprintf(`ip addr list dev "%s"`, lo))
	lst, _, _ := cmdRun(t, fmt.Sprintf(`ip addr lst dev "%s"`, lo))
	assert.Equal(t, base, list)
	assert.Equal(t, base, lst)
}

// TestIPAddrShowNormalFormat verifies the standard multi-line output format.
func TestIPAddrShowNormalFormat(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	// Interface header line: "N: lo0: <...> mtu ... state ... group default qlen 1000"
	assert.Contains(t, stdout, lo+":")
	assert.Contains(t, stdout, "mtu ")
	assert.Contains(t, stdout, "state ")
	assert.Contains(t, stdout, "group default")
	// Link line: "    link/loopback ..."
	assert.Contains(t, stdout, "link/loopback")
	// Addr line: "    inet 127.0.0.1/8 scope host ..."
	assert.Contains(t, stdout, "    inet 127.0.0.1")
	assert.Contains(t, stdout, "scope host")
	// valid_lft line
	assert.Contains(t, stdout, "valid_lft forever preferred_lft forever")
}

// TestIPAddrShowBrief verifies --brief produces a compact tabular format.
func TestIPAddrShowBrief(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip --brief addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	// Brief format: "lo0              UNKNOWN        127.0.0.1/8 ..."
	assert.Contains(t, stdout, lo)
	assert.Contains(t, stdout, "127.0.0.1")
	// Should NOT contain the multi-line format markers
	assert.NotContains(t, stdout, "link/loopback")
	assert.NotContains(t, stdout, "valid_lft")
}

// TestIPAddrShowOneline verifies -o produces one line per address with backslash continuation.
func TestIPAddrShowOneline(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip -o addr show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "inet 127.0.0.1")
	assert.Contains(t, stdout, "valid_lft forever")
	// In oneline mode, each address record is on one line separated by \
	assert.Contains(t, stdout, `\`)
	// No standalone "link/loopback" line (it's merged or omitted in oneline addr mode)
}

// ============================================================================
// ip link show — success cases
// ============================================================================

// TestIPLinkShowExitsZero verifies "ip link show" runs successfully.
func TestIPLinkShowExitsZero(t *testing.T) {
	stdout, _, code := cmdRun(t, "ip link show")
	assert.Equal(t, 0, code)
	assert.NotEmpty(t, stdout)
}

// TestIPLinkShowDevLoopback verifies link show filtered by loopback interface.
func TestIPLinkShowDevLoopback(t *testing.T) {
	lo := loopbackName(t)
	stdout, stderr, code := cmdRun(t, fmt.Sprintf(`ip link show dev "%s"`, lo))
	require.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, lo)
	assert.Contains(t, stdout, "LOOPBACK")
	assert.Contains(t, stdout, "link/loopback")
}

// TestIPLinkShowNormalFormat verifies the standard multi-line link output format.
func TestIPLinkShowNormalFormat(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip link show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	// Header: "1: lo: <LOOPBACK,...> mtu ... state UNKNOWN mode DEFAULT group default qlen 1000"
	assert.Contains(t, stdout, lo+":")
	assert.Contains(t, stdout, "mtu ")
	assert.Contains(t, stdout, "mode DEFAULT")
	assert.Contains(t, stdout, "group default")
	// Link line
	assert.Contains(t, stdout, "link/loopback")
	// No address lines (those are for addr show)
	assert.NotContains(t, stdout, "inet ")
}

// TestIPLinkShowBrief verifies --brief produces tabular link format.
func TestIPLinkShowBrief(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip --brief link show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, lo)
	assert.Contains(t, stdout, "UNKNOWN")
	assert.Contains(t, stdout, "LOOPBACK")
}

// TestIPLinkShowOneline verifies -o produces single-line link output with backslash.
func TestIPLinkShowOneline(t *testing.T) {
	lo := loopbackName(t)
	stdout, _, code := cmdRun(t, fmt.Sprintf(`ip -o link show dev "%s"`, lo))
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, lo)
	assert.Contains(t, stdout, "link/loopback")
	// Oneline mode uses \ to join header and link lines.
	assert.Contains(t, stdout, `\`)
	// Everything should be on one line (no bare newline before "link/loopback").
	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	assert.Equal(t, 1, len(lines), "expected one line in oneline output, got: %q", stdout)
}

// TestIPLinkShowAllInterfacesContainsState verifies link show for all interfaces
// contains at least one state label (UP, DOWN, or UNKNOWN), exercising the
// non-loopback branches of ifaceState.
func TestIPLinkShowAllInterfacesContainsState(t *testing.T) {
	stdout, _, code := cmdRun(t, "ip link show")
	assert.Equal(t, 0, code)
	hasState := strings.Contains(stdout, " state UP ") ||
		strings.Contains(stdout, " state DOWN ") ||
		strings.Contains(stdout, " state UNKNOWN ")
	assert.True(t, hasState, "expected at least one state line in: %q", stdout)
}

// TestIPAddrShowUnknownTokenRejected verifies that unknown tokens after "show"
// are rejected with exit 1 (prevents silently ignoring mistyped commands).
func TestIPAddrShowUnknownTokenRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr show type veth")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unknown token")
}

// TestIPLinkShowUnknownTokenRejected verifies that unknown tokens after "show"
// are rejected with exit 1 for the link subcommand.
func TestIPLinkShowUnknownTokenRejected(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip link show type ether")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unknown token")
}

// ============================================================================
// ip — error cases
// ============================================================================

// TestIPNoArgs verifies ip with no arguments exits 1 with "object required".
func TestIPNoArgs(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "object required")
}

// TestIPUnknownObject verifies ip with unknown object exits 1 with error.
func TestIPUnknownObject(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip foobar-unknown-object")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPUnknownInterface verifies dev <nonexistent> exits 1 with "cannot find device".
func TestIPUnknownInterface(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr show dev nonexistent-xyzzy-99")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cannot find device")
}

// TestIPUnknownInterfaceLink verifies link show for nonexistent dev also fails.
func TestIPUnknownInterfaceLink(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip link show dev nonexistent-xyzzy-99")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "cannot find device")
}

// TestIPMissingDevArgument verifies "ip addr show dev" with no name exits 1.
func TestIPMissingDevArgument(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr show dev")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "requires an interface name")
}

// ============================================================================
// ip — write operations blocked
// ============================================================================

// TestIPAddrAddBlocked verifies "ip addr add" is blocked as a write operation.
func TestIPAddrAddBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr add 10.0.0.1/24 dev lo")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPAddrDelBlocked verifies "ip addr del" is blocked.
func TestIPAddrDelBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr del 10.0.0.1/24 dev lo")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPAddrFlushBlocked verifies "ip addr flush" is blocked.
func TestIPAddrFlushBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr flush dev lo")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPAddrChangeBlocked verifies "ip addr change" is blocked.
func TestIPAddrChangeBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip addr change 10.0.0.1/24 dev lo")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPLinkSetBlocked verifies "ip link set" is blocked.
func TestIPLinkSetBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip link set lo up")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPLinkDelBlocked verifies "ip link del" is blocked.
func TestIPLinkDelBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip link del lo")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// ============================================================================
// ip — GTFOBins security: blocked vectors
// ============================================================================

// TestIPNetnsBlocked verifies "ip netns" is blocked to prevent shell escape.
// GTFOBins: ip netns exec <ns> <cmd> spawns a shell.
func TestIPNetnsBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip netns exec mynamespace sh")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "blocked")
}

// TestIPBatchFlagBlocked verifies -b (batch) flag is rejected as unknown.
// GTFOBins: ip -b FILE reads and executes ip commands from a file.
func TestIPBatchFlagBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip -b /tmp/cmds addr show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPForceFlagBlocked verifies --force is rejected as unknown flag.
// GTFOBins: --force suppresses errors in batch mode.
func TestIPForceFlagBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip --force addr show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPNetnsGlobalFlagBlocked verifies -n (netns switch) is rejected.
// GTFOBins: ip -n NETNS switches to a network namespace.
func TestIPNetnsGlobalFlagBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip -n mynamespace addr show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// TestIPBCapitalFlagBlocked verifies -B (batch stdin) is rejected.
func TestIPBCapitalFlagBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip -B addr show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "ip:")
}

// ============================================================================
// ip --help
// ============================================================================

// TestIPHelp verifies --help prints usage to stdout and exits 0.
func TestIPHelp(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ip --help")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "addr")
	assert.Contains(t, stdout, "link")
	assert.Contains(t, stdout, "blocked")
}

// TestIPShortHelp verifies -h also prints usage and exits 0.
func TestIPShortHelp(t *testing.T) {
	stdout, stderr, code := cmdRun(t, "ip -h")
	assert.Equal(t, 0, code)
	assert.Empty(t, stderr)
	assert.Contains(t, stdout, "Usage:")
}

// ============================================================================
// ip — context cancellation (DoS prevention)
// ============================================================================

// TestIPAddrContextCancellation verifies ip addr show respects context cancellation.
func TestIPAddrContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "ip addr show", "")
	// May succeed or be cancelled but must not hang.
	assert.True(t, code == 0 || code == 1)
}

// TestIPLinkContextCancellation verifies ip link show respects context cancellation.
func TestIPLinkContextCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "ip link show", "")
	assert.True(t, code == 0 || code == 1)
}
