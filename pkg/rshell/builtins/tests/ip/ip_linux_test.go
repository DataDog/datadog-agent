// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package ip_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ipcmd "github.com/DataDog/datadog-agent/pkg/rshell/builtins/ip"
)

// procNetRouteMu serializes mutations of ipcmd.ProcNetRoutePath across all
// test files in this package (unit tests, pentest tests, fuzz tests).
// Any code that writes to ProcNetRoutePath must hold this lock for the
// duration of the test to prevent data races if tests are run in parallel.
// Tests in this package must NOT call t.Parallel() — doing so would cause
// test goroutines to block indefinitely on procNetRouteMu.Lock() while
// another test holds the lock. The -race detector will surface any accidental
// concurrent mutation if this rule is violated.
var procNetRouteMu sync.Mutex

// syntheticProcNetRoute is a realistic /proc/net/route file with:
//   - A default route via 192.168.1.1 on eth0 (metric 100)
//   - A network route for 192.168.1.0/24 on eth0 (metric 100)
//   - A loopback route for 127.0.0.0/8 on lo (metric 0)
//   - A down route (RTF_UP not set) — should be skipped
//
// Encoding: IPs are little-endian uint32 hex.
//
//	192.168.1.1  = 0x0101A8C0
//	192.168.1.0  = 0x0001A8C0
//	255.255.255.0 = 0x00FFFFFF
//	127.0.0.0    = 0x0000007F
//	255.0.0.0    = 0x000000FF
const syntheticProcNetRoute = `Iface	Destination	Gateway	Flags	RefCnt	Use	Metric	Mask	MTU	Window	IRTT
eth0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
eth0	0001A8C0	00000000	0001	0	0	100	00FFFFFF	0	0	0
lo	0000007F	00000000	0001	0	0	0	000000FF	0	0	0
eth0	0002A8C0	00000000	0000	0	0	200	00FFFFFF	0	0	0
`

// writeProcNetRoute writes synthetic /proc/net/route content to a temp directory
// tree (dir/net/route), patches ipcmd.ProcNetRoutePath to the temp directory,
// and restores the original path via t.Cleanup.
//
// It acquires procNetRouteMu for the duration of the test to prevent data races.
// Tests in this package must NOT call t.Parallel() — the mutex serializes
// ProcNetRoutePath mutations, but parallel execution would deadlock waiting for
// the lock that the currently-running test already holds.
//
// The procnet package opens procPath/net/route directly with os.Open, so no
// AllowedPaths sandbox configuration is needed — use cmdRun for all route tests.
func writeProcNetRoute(t *testing.T, content string) {
	t.Helper()
	dir := t.TempDir()
	netDir := filepath.Join(dir, "net")
	require.NoError(t, os.MkdirAll(netDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(netDir, "route"), []byte(content), 0o644))
	procNetRouteMu.Lock()
	orig := ipcmd.ProcNetRoutePath
	ipcmd.ProcNetRoutePath = dir
	t.Cleanup(func() {
		ipcmd.ProcNetRoutePath = orig
		procNetRouteMu.Unlock()
	})
}

// ============================================================================
// ip route show / list
// ============================================================================

// TestIPRouteShowDefault verifies "ip route show" outputs the default route.
func TestIPRouteShowDefault(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, stderr, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "default via 192.168.1.1 dev eth0 metric 100")
}

// TestIPRouteShowNetworkRoute verifies "ip route show" outputs network routes.
func TestIPRouteShowNetworkRoute(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.0/24 dev eth0 metric 100")
}

// TestIPRouteShowLoopback verifies the loopback route appears with no gateway.
func TestIPRouteShowLoopback(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "127.0.0.0/8 dev lo")
	assert.NotContains(t, stdout, "127.0.0.0/8 dev lo via") // no gateway
}

// TestIPRouteShowZeroMetricOmitted verifies that metric 0 is not printed.
func TestIPRouteShowZeroMetricOmitted(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	// lo has metric 0 — it should not appear in the lo line
	assert.NotContains(t, stdout, "127.0.0.0/8 dev lo metric 0")
}

// TestIPRouteShowDownRouteSkipped verifies routes without RTF_UP are excluded.
func TestIPRouteShowDownRouteSkipped(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	// The 192.168.2.0/24 route has flags=0x0000 (RTF_UP not set) — must be absent.
	assert.NotContains(t, stdout, "192.168.2.0")
}

// TestIPRouteShowRejectRoute verifies that RTF_REJECT routes (flags & 0x0200)
// are included in "ip route show" output with the "unreachable" prefix and no
// "dev" field, matching real ip-route(8) behaviour.
//
// In /proc/net/route, an unreachable route has flags=0x0201 (RTF_UP|RTF_REJECT)
// and the interface name "*".  The little-endian encoding of 10.0.0.0 is
// 0x0000000A (byte 0 = 10 = 0x0A at LSB).
func TestIPRouteShowRejectRoute(t *testing.T) {
	// 10.0.0.0/8: Dest=0x0000000A, Mask=0x000000FF, flags=0x0201 (RTF_UP|RTF_REJECT)
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"*\t0000000A\t00000000\t0201\t0\t0\t0\t000000FF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "unreachable 10.0.0.0/8")
	assert.NotContains(t, stdout, "unreachable 10.0.0.0/8 dev") // no "dev" for reject routes
	assert.Contains(t, stdout, "default via 192.168.1.1 dev eth0")
}

// TestIPRouteGetRejectRouteUnreachable verifies that when a RTF_REJECT route
// is the best match for the destination, "ip route get" returns exit 1 with a
// "network unreachable" error instead of reporting the less-specific fallback.
func TestIPRouteGetRejectRouteUnreachable(t *testing.T) {
	// 10.0.0.0/8 is an unreachable route; default is via 192.168.1.1.
	// A lookup for 10.1.2.3 should match the more-specific /8 reject route
	// and return "network unreachable", not the default.
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"*\t0000000A\t00000000\t0201\t0\t0\t0\t000000FF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	_, stderr, code := cmdRun(t, "ip route get 10.1.2.3")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unreachable")
}

// TestIPRouteGetNonRejectRouteStillWorks verifies that when a RTF_REJECT route
// exists but does not match, the normal route is still returned.
func TestIPRouteGetNonRejectRouteStillWorks(t *testing.T) {
	// 10.0.0.0/8 is unreachable, but 8.8.8.8 falls through to the default route.
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"*\t0000000A\t00000000\t0201\t0\t0\t0\t000000FF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, stderr, code := cmdRun(t, "ip route get 8.8.8.8")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "via 192.168.1.1")
}

// TestIPRouteShowZeroDestNonZeroMaskNotDefault verifies that a route with
// Dest=0 but a non-zero mask (e.g. 0.0.0.0/8) is NOT formatted as "default".
// Only a /0 route (Dest=0, Mask=0) should use the "default" keyword.
func TestIPRouteShowZeroDestNonZeroMaskNotDefault(t *testing.T) {
	// 0.0.0.0/8: Dest=0x00000000, Mask=0x000000FF (255.0.0.0 little-endian)
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t00000000\t0001\t0\t0\t0\t000000FF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "0.0.0.0/8 dev eth0")
	assert.NotContains(t, stdout, "default")
}

// TestIPRouteSubcmdCaseSensitive verifies that route subcommands are
// case-sensitive, matching real iproute2 behavior.
func TestIPRouteSubcmdCaseSensitive(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)

	_, stderr, code := cmdRun(t, "ip route SHOW")
	assert.Equal(t, 255, code, "uppercase SHOW should be rejected")
	assert.Contains(t, stderr, "is unknown, try")

	_, stderr2, code2 := cmdRun(t, "ip route LIST")
	assert.Equal(t, 255, code2, "uppercase LIST should be rejected")
	assert.Contains(t, stderr2, "is unknown, try")
}

// TestIPRouteListAliasForShow verifies "ip route list" is an alias for show.
func TestIPRouteListAliasForShow(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	show, _, code1 := cmdRun(t, "ip route show")
	list, _, code2 := cmdRun(t, "ip route list")
	assert.Equal(t, 0, code1)
	assert.Equal(t, 0, code2)
	assert.Equal(t, show, list)
}

// TestIPRouteShowDefaultRouteAlias verifies "ip route" (no subcommand) defaults to show.
func TestIPRouteShowDefaultRouteAlias(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	withShow, _, code1 := cmdRun(t, "ip route show")
	withoutSub, _, code2 := cmdRun(t, "ip route")
	assert.Equal(t, 0, code1)
	assert.Equal(t, 0, code2)
	assert.Equal(t, withShow, withoutSub)
}

// TestIPRouteShowEmptyTable verifies "ip route show" on an empty table outputs nothing.
func TestIPRouteShowEmptyTable(t *testing.T) {
	// Only the header row, no data rows.
	writeProcNetRoute(t, "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n")
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Empty(t, stdout)
}

// TestIPRouteShowDefaultOnly verifies output with only a default route.
func TestIPRouteShowDefaultOnly(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Equal(t, "default via 192.168.1.1 dev eth0 metric 100\n", stdout)
}

// TestIPRouteShowMalformedLinesSkipped verifies malformed lines are skipped
// without crashing.
func TestIPRouteShowMalformedLinesSkipped(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"not-enough-fields\n" + // too few fields
		"eth0\tZZZZZZZZ\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" + // invalid hex dest
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" // valid default
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "default")
}

// TestIPRouteShowLargeMetric verifies a large metric value is printed correctly.
func TestIPRouteShowLargeMetric(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t4294967295\t00000000\t0\t0\t0\n" // metric near max uint32
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "metric 4294967295")
}

// TestIPRouteShowTrailingArgRejected verifies that unsupported trailing
// arguments to "ip route show" are rejected with exit 1.
func TestIPRouteShowTrailingArgRejected(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip route show from 1.1.1.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unsupported argument")
}

// TestIPRouteListTrailingArgRejected verifies "ip route list" also rejects
// unsupported trailing arguments.
func TestIPRouteListTrailingArgRejected(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip route list from 1.1.1.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unsupported argument")
}

// ============================================================================
// ip route get
// ============================================================================

// TestIPRouteGetDefaultRoute verifies get selects the default route for an
// address with no more-specific match.
func TestIPRouteGetDefaultRoute(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, stderr, code := cmdRun(t, "ip route get 10.0.0.1")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "10.0.0.1")
	assert.Contains(t, stdout, "via 192.168.1.1")
	assert.Contains(t, stdout, "dev eth0")
}

// TestIPRouteGetNetworkRoute verifies get selects the network route for an
// address within the 192.168.1.0/24 subnet (more specific than default).
func TestIPRouteGetNetworkRoute(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, stderr, code := cmdRun(t, "ip route get 192.168.1.50")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "192.168.1.50")
	// No "via" for directly connected route
	assert.NotContains(t, stdout, "via")
	assert.Contains(t, stdout, "dev eth0")
}

// TestIPRouteGetLoopback verifies get selects the loopback route for 127.x.x.x.
func TestIPRouteGetLoopback(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	stdout, stderr, code := cmdRun(t, "ip route get 127.0.0.1")
	assert.Equal(t, 0, code, "stderr: %s", stderr)
	assert.Contains(t, stdout, "127.0.0.1")
	assert.Contains(t, stdout, "dev lo")
}

// TestIPRouteGetUnreachable verifies get returns exit 1 when no route matches.
func TestIPRouteGetUnreachable(t *testing.T) {
	// Only a /24 network route — no default.
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	_, stderr, code := cmdRun(t, "ip route get 10.0.0.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "unreachable")
}

// TestIPRouteGetLongestPrefixMatch verifies that the most-specific prefix wins
// when both a /24 and a /16 route match.
func TestIPRouteGetLongestPrefixMatch(t *testing.T) {
	// 10.1.2.0/24 via gw1 and 10.1.0.0/16 via gw2 — address 10.1.2.5 should
	// match the /24 (longer prefix).
	//   10.1.2.0   = 0x0002010A (little-endian)
	//   255.255.255.0 = 0x00FFFFFF
	//   10.1.0.0   = 0x0000010A
	//   255.255.0.0   = 0x0000FFFF
	//   gw1 = 10.0.0.1 = 0x0100000A
	//   gw2 = 10.0.0.2 = 0x0200000A
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t0002010A\t0100000A\t0003\t0\t0\t0\t00FFFFFF\t0\t0\t0\n" +
		"eth0\t0000010A\t0200000A\t0003\t0\t0\t0\t0000FFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route get 10.1.2.5")
	assert.Equal(t, 0, code)
	// Must select the /24 gateway (10.0.0.1), not the /16 (10.0.0.2).
	assert.Contains(t, stdout, "via 10.0.0.1")
	assert.NotContains(t, stdout, "via 10.0.0.2")
}

// TestIPRouteGetMetricTieBreak verifies that when two routes have equal prefix
// length, the one with the lower metric is preferred.
func TestIPRouteGetMetricTieBreak(t *testing.T) {
	// Two default routes (/0) — one with metric 100 (gw1), one with metric 50 (gw2).
	// gw1 = 10.0.0.1 = 0x0100000A, gw2 = 10.0.0.2 = 0x0200000A
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0100000A\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth1\t00000000\t0200000A\t0003\t0\t0\t50\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route get 8.8.8.8")
	assert.Equal(t, 0, code)
	// Must select the lower-metric gateway (10.0.0.2 via eth1, metric 50).
	assert.Contains(t, stdout, "via 10.0.0.2")
	assert.NotContains(t, stdout, "via 10.0.0.1")
}

// TestIPRouteGetInvalidAddr verifies get with a non-IP argument returns exit 1.
// Input validation happens before file access, so no AllowedPaths needed.
func TestIPRouteGetInvalidAddr(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip route get not-an-ip")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid address")
}

// TestIPRouteGetMissingAddr verifies "ip route get" with no address returns exit 1.
func TestIPRouteGetMissingAddr(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route get")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "missing address")
}

// ============================================================================
// ip route — write operations blocked
// ============================================================================

// TestIPRouteAddBlocked verifies "ip route add" is blocked.
func TestIPRouteAddBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route add 10.0.0.0/8 via 192.168.1.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteDelBlocked verifies "ip route del" is blocked.
func TestIPRouteDelBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route del default")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteDeleteAliasBlocked verifies "ip route delete" is blocked.
func TestIPRouteDeleteAliasBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route delete default")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteChangeBlocked verifies "ip route change" is blocked.
func TestIPRouteChangeBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route change default via 10.0.0.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteReplaceBlocked verifies "ip route replace" is blocked.
func TestIPRouteReplaceBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route replace default via 10.0.0.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteFlushBlocked verifies "ip route flush" is blocked.
func TestIPRouteFlushBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route flush")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteSaveBlocked verifies "ip route save" is blocked.
func TestIPRouteSaveBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route save")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteRestoreBlocked verifies "ip route restore" is blocked.
func TestIPRouteRestoreBlocked(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route restore")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "write operations are not permitted")
}

// TestIPRouteUnknownSubcommand verifies an unknown subcommand exits 255 with iproute2-compatible message.
func TestIPRouteUnknownSubcommand(t *testing.T) {
	_, stderr, code := cmdRun(t, "ip route unknowncmd")
	assert.Equal(t, 255, code)
	assert.Contains(t, stderr, "is unknown, try")
}

// ============================================================================
// ip -6 route — blocked on route
// ============================================================================

// TestIPIPv6RouteBlocked verifies "-6 route show" returns exit 1 with a clear
// error (IPv6 routing is not supported via /proc/net/route).
func TestIPIPv6RouteBlocked(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip -6 route show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "IPv6")
}

// ============================================================================
// ip route — max-routes cap (memory safety)
// ============================================================================

// TestIPRouteMaxRoutesCap verifies that ip route show fails with an error when
// the route table exceeds MaxRoutes entries. Silently truncating would cause
// ip route get to compute LPM on incomplete data and return a wrong next-hop.
func TestIPRouteMaxRoutesCap(t *testing.T) {
	// Build a file with 15 000 route entries (> maxRoutes=10000).
	var b []byte
	b = append(b, "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n"...)
	row := "eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	for range 15_000 {
		b = append(b, row...)
	}
	writeProcNetRoute(t, string(b))
	_, stderr, code := cmdRun(t, "ip route show")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "MaxRoutes")
}

// ============================================================================
// ip route — coverage for parseRouteEntry failure paths
// ============================================================================

// TestIPRouteParseEntryExactlyElevenFields verifies that a valid line with
// exactly 11 fields (the minimum) is accepted.
func TestIPRouteParseEntryExactlyElevenFields(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "default")
}

// TestIPRouteParseEntryBadGateway verifies a line with an invalid hex gateway
// is skipped without crashing.
func TestIPRouteParseEntryBadGateway(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\tZZZZZZZZ\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.0/24") // valid line still appears
	assert.NotContains(t, stdout, "default")     // bad line skipped
}

// TestIPRouteParseEntryBadFlags verifies a line with invalid hex flags is skipped.
func TestIPRouteParseEntryBadFlags(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\tZZZZ\t0\t0\t100\t00000000\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.0/24")
}

// TestIPRouteParseEntryBadMetric verifies a line with invalid decimal metric is skipped.
func TestIPRouteParseEntryBadMetric(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\tNAN\t00000000\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.0/24")
}

// TestIPRouteParseEntryBadMask verifies a line with invalid hex mask is skipped.
func TestIPRouteParseEntryBadMask(t *testing.T) {
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\tZZZZZZZZ\t0\t0\t0\n" +
		"eth0\t0001A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.0/24")
}

// TestIPRouteShowNonOctetMasks verifies that non-octet-aligned CIDR masks
// (e.g. /25, /17, /20, /28) from /proc/net/route are accepted and displayed
// correctly.  These are stored in little-endian byte order, so /25
// (255.255.255.128) appears as 0x80FFFFFF — IsContiguousMask must not reject
// them by assuming byte-aligned LSB-contiguous form.
func TestIPRouteShowNonOctetMasks(t *testing.T) {
	// /25 = 255.255.255.128 -> little-endian 0x80FFFFFF
	// /17 = 255.255.128.0   -> little-endian 0x0080FFFF
	// /20 = 255.255.240.0   -> little-endian 0x00F0FFFF
	// /28 = 255.255.255.240 -> little-endian 0xF0FFFFFF
	//
	// Dest for 192.168.1.128/25: 192.168.1.128 in /proc LE encoding:
	//   first octet (192=0xC0) in LSB, last octet (128=0x80) in MSB
	//   -> byte0=0xC0, byte1=0xA8, byte2=0x01, byte3=0x80 -> 0x8001A8C0
	// Dest for 10.1.0.0/17: 10.1.0.0 LE = 0x0000010A
	// Dest for 10.1.0.0/20: 10.1.0.0 LE = 0x0000010A (same dest, different mask)
	// Dest for 192.168.1.240/28: 192.168.1.240 LE = 0xF001A8C0

	// /25 route: 192.168.1.128/25 dev eth0 — flags=0x0001 (UP, no GW)
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth0\t8001A8C0\t00000000\t0001\t0\t0\t0\t80FFFFFF\t0\t0\t0\n" +
		"eth0\t0000010A\t00000000\t0001\t0\t0\t0\t0080FFFF\t0\t0\t0\n" +
		"eth0\t0000010A\t00000000\t0001\t0\t0\t10\t00F0FFFF\t0\t0\t0\n" +
		"eth0\tF001A8C0\t00000000\t0001\t0\t0\t0\tF0FFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "192.168.1.128/25", "expected /25 route in output")
	assert.Contains(t, stdout, "/17", "expected /17 route in output")
	assert.Contains(t, stdout, "/20", "expected /20 route in output")
	assert.Contains(t, stdout, "/28", "expected /28 route in output")
}

// TestIPRouteGetHostRoute verifies a /32 route (exact host match) wins over broader
// routes via longest-prefix-match (popcount of 0xFFFFFFFF = 32 bits).
func TestIPRouteGetHostRoute(t *testing.T) {
	//   host route: 10.0.0.1/32 (mask=0xFFFFFFFF) direct via eth1
	//   default:    0.0.0.0/0   via gw 192.168.1.1 via eth0
	//   10.0.0.1 = 0x0100000A (little-endian)
	//   255.255.255.255 = 0xFFFFFFFF
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth1\t0100000A\t00000000\t0001\t0\t0\t0\tFFFFFFFF\t0\t0\t0\n" +
		"eth0\t00000000\t0101A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route get 10.0.0.1")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "dev eth1")
	assert.NotContains(t, stdout, "via 192.168.1.1")
}

// TestIPRouteGetInvalidAddrEmpty verifies empty string is rejected.
func TestIPRouteGetInvalidAddrEmpty(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, `ip route get ""`)
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid address")
}

// TestIPRouteGetInvalidAddrTooFewOctets verifies "192.168.1" (no 4th octet) is rejected.
func TestIPRouteGetInvalidAddrTooFewOctets(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip route get 192.168.1")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid address")
}

// TestIPRouteGetInvalidAddrOctetOverflow verifies an octet > 255 is rejected.
func TestIPRouteGetInvalidAddrOctetOverflow(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	_, stderr, code := cmdRun(t, "ip route get 192.168.1.256")
	assert.Equal(t, 1, code)
	assert.Contains(t, stderr, "invalid address")
}

// ============================================================================
// ip route — context cancellation (DoS prevention)
// ============================================================================

// TestIPRouteShowContextCancellation verifies "ip route show" honours context
// cancellation and does not hang when the context is cancelled.
func TestIPRouteShowContextCancellation(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "ip route show", "")
	assert.Equal(t, 0, code, "expected exit 0 on context cancellation, got %d", code)
}

// TestIPRouteGetContextCancellation verifies "ip route get" honours context
// cancellation and does not hang when the context is cancelled.
func TestIPRouteGetContextCancellation(t *testing.T) {
	writeProcNetRoute(t, syntheticProcNetRoute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, _, code := runScriptCtx(ctx, t, "ip route get 10.0.0.1", "")
	assert.True(t, code == 0 || code == 1, "expected exit 0 or 1, got %d", code)
}

// ============================================================================
// ip route show — host route /32 and reject route metric formatting
// ============================================================================

// TestIPRouteShowHostRouteNoSlash32 verifies that a /32 host route is displayed
// without the "/32" suffix, matching real ip-route(8) output.
func TestIPRouteShowHostRouteNoSlash32(t *testing.T) {
	// 10.0.0.1/32: Dest=0x0100000A, Mask=0xFFFFFFFF (255.255.255.255 little-endian)
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"eth1\t0100000A\t00000000\t0001\t0\t0\t0\tFFFFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	// Host routes must appear without /32 — "10.0.0.1 dev eth1", not "10.0.0.1/32 dev eth1".
	assert.Contains(t, stdout, "10.0.0.1 dev eth1")
	assert.NotContains(t, stdout, "10.0.0.1/32")
}

// TestIPRouteShowRejectHostRouteNoSlash32 verifies that an unreachable /32 host
// route is displayed without the "/32" suffix.
func TestIPRouteShowRejectHostRouteNoSlash32(t *testing.T) {
	// unreachable 10.0.0.1/32: Dest=0x0100000A, Mask=0xFFFFFFFF, flags=0x0201
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"*\t0100000A\t00000000\t0201\t0\t0\t0\tFFFFFFFF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	// Reject host routes: "unreachable 10.0.0.1", not "unreachable 10.0.0.1/32".
	assert.Contains(t, stdout, "unreachable 10.0.0.1")
	assert.NotContains(t, stdout, "unreachable 10.0.0.1/32")
}

// TestIPRouteShowRejectRouteWithMetric verifies that a RTF_REJECT route with a
// non-zero metric includes the metric in its output.
func TestIPRouteShowRejectRouteWithMetric(t *testing.T) {
	// unreachable 10.0.0.0/8 with metric 50
	content := "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n" +
		"*\t0000000A\t00000000\t0201\t0\t0\t50\t000000FF\t0\t0\t0\n"
	writeProcNetRoute(t, content)
	stdout, _, code := cmdRun(t, "ip route show")
	assert.Equal(t, 0, code)
	assert.Contains(t, stdout, "unreachable 10.0.0.0/8 metric 50")
}
