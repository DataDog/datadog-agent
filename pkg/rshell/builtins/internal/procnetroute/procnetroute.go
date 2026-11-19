// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package procnetroute reads the Linux IPv4 routing table from /proc/net/route.
//
// This package is in builtins/internal/ and is therefore exempt from the
// builtinAllowedSymbols allowlist check. It may use OS-specific APIs freely.
//
// # Sandbox bypass
//
// ReadRoutes intentionally bypasses the AllowedPaths sandbox (callCtx.OpenFile)
// and calls os.Open directly. This is safe because procPath is always a
// kernel-managed pseudo-filesystem root (/proc by default) that is hardcoded
// by the caller — it is never derived from user-supplied input and cannot be
// redirected by a shell script. The caller is responsible for ensuring that
// procPath remains a safe, non-user-controlled path.
//
// /proc/net/route format (tab-separated, one route per line after the header):
//
//	Iface  Destination  Gateway  Flags  RefCnt  Use  Metric  Mask  MTU  Window  IRTT
//	eth0   00000000     0101A8C0 0003   0       0    100     00000000 0  0       0
//
// All IP fields are little-endian uint32 hex: 192.168.1.1 is encoded as
// 0x0101A8C0 (first octet in the least-significant byte).
package procnetroute

import (
	"context"
	"fmt"
	"math/bits"
	"path/filepath"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procpath"
)

// DefaultProcPath is the default proc filesystem root.
// ReadRoutes appends "net/route" to this path to locate the routing table.
const DefaultProcPath = procpath.Default

// MaxRoutes caps the number of UP route entries retained in memory to prevent
// memory exhaustion.
const MaxRoutes = 10_000

// MaxTotalLines caps the total number of lines (UP + non-UP + malformed)
// scanned per ReadRoutes call. This bounds CPU time for pathological
// /proc/net/route files with many non-UP/malformed lines before MaxRoutes UP
// entries are found. MaxRoutes is the memory guard; MaxTotalLines is the
// scan-time guard.
const MaxTotalLines = MaxRoutes * 10 // 100 000 lines

// MaxLineBytes is the per-line buffer cap for the route-table scanner.
// If any line in the route file exceeds this limit the scanner returns
// bufio.ErrTooLong and ReadRoutes returns an error; processing is aborted
// rather than allowing unbounded allocation.
const MaxLineBytes = 1 << 20 // 1 MiB

// Routing-table flags (from linux/route.h).
const (
	FlagUp      = uint32(0x0001) // RTF_UP
	FlagGateway = uint32(0x0002) // RTF_GATEWAY
	FlagReject  = uint32(0x0200) // RTF_REJECT — kernel will refuse to route to this destination
)

// Route holds a parsed entry from /proc/net/route.
// IP fields use the same little-endian uint32 encoding as /proc/net/route:
// for 192.168.1.1 the stored value is 0x0101A8C0 and
// HexToIPStr(0x0101A8C0) returns "192.168.1.1".
type Route struct {
	Iface  string
	Dest   uint32
	GW     uint32
	Flags  uint32
	Metric uint32
	Mask   uint32
}

// ReadRoutes opens procPath/net/route and returns all UP route entries.
// procPath is the proc filesystem root (e.g. DefaultProcPath or a test override).
// It is implemented on Linux and returns an error on other platforms.
//
// Sandbox bypass: this function calls os.Open directly, bypassing the
// AllowedPaths sandbox enforced by callCtx.OpenFile. This is intentional —
// procPath must always be a safe, hardcoded kernel pseudo-filesystem path
// (e.g. /proc) that is not controllable from user scripts. Never pass a
// path derived from user input.
//
// Safety invariants:
//
//	(a) /proc-prefix requirement: all callers MUST pass a path that starts with
//	    /proc. No runtime assertion enforces this — tests override procPath with
//	    a temp-directory tree to inject synthetic route data, so a /proc-prefix
//	    check would break those tests. This invariant is caller-enforced only.
//
//	(b) No ".." components: enforced at runtime by the strings.Contains check
//	    below. The check is applied to the ORIGINAL path (before filepath.Clean)
//	    so traversal sequences like "/proc/../etc/passwd" are caught — after
//	    Clean, such a path becomes "/etc/passwd" and no longer contains "..".
//	    Temp-directory overrides used by tests never contain "..".
//	    Note: the check is deliberately conservative: any path whose component
//	    names happen to contain ".." as a substring (e.g. "/proc..backup") is
//	    also rejected. This is a theoretical false-positive that never occurs in
//	    practice since procPath is always "/proc" or a t.TempDir() path.
func ReadRoutes(ctx context.Context, procPath string) ([]Route, error) {
	if strings.Contains(procPath, "..") {
		return nil, fmt.Errorf("procnetroute: unsafe procPath %q (must not contain \"..\" components)", procPath)
	}
	return readRoutes(ctx, filepath.Clean(procPath))
}

// HexToIPStr converts a /proc/net/route little-endian uint32 to dotted-decimal.
// The encoding stores the first octet in the least-significant byte:
// 192.168.1.1 is encoded as 0x0101A8C0, and HexToIPStr(0x0101A8C0) = "192.168.1.1".
func HexToIPStr(val uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d",
		val&0xFF,
		(val>>8)&0xFF,
		(val>>16)&0xFF,
		(val>>24)&0xFF,
	)
}

// Popcount returns the number of set bits in v (used for prefix length).
func Popcount(v uint32) int {
	return bits.OnesCount32(v)
}

// IsContiguousMask reports whether v is a valid CIDR subnet mask in the
// little-endian encoding used by /proc/net/route, where the first octet is
// stored in the least-significant byte.  For example:
//   - /24 (255.255.255.0)   is stored as 0x00FFFFFF
//   - /25 (255.255.255.128) is stored as 0x80FFFFFF
//   - /28 (255.255.255.240) is stored as 0xF0FFFFFF
//
// Non-contiguous masks (e.g. 0xF0F0F0F0) are not valid CIDR prefixes and
// would produce misleading output from LongestPrefixMatch and formatRoute.
func IsContiguousMask(v uint32) bool {
	// Convert from little-endian (/proc encoding) to network byte order, then
	// verify the result is a valid CIDR mask (all 1-bits from MSB then all 0-bits).
	// A network-order mask's complement is of the form (1<<n)−1, which satisfies
	// complement & (complement+1) == 0.
	// This covers v=0 (/0, complement=0xFFFFFFFF) and v=0xFFFFFFFF (/32, complement=0).
	netOrder := bits.ReverseBytes32(v)
	complement := ^netOrder
	return complement&(complement+1) == 0
}

// LongestPrefixMatch returns the route that best matches addr by
// longest-prefix-match with metric as a tie-breaker (lower metric wins),
// or nil if no route matches.
func LongestPrefixMatch(routes []Route, addr uint32) *Route {
	var best *Route
	bestBits := -1
	for i := range routes {
		r := &routes[i]
		if addr&r.Mask == r.Dest {
			prefixLen := Popcount(r.Mask)
			// Longer prefix wins; equal-prefix ties break on lower metric;
			// equal-prefix equal-metric ties keep the first entry (kernel order).
			if prefixLen > bestBits || (best != nil && prefixLen == bestBits && r.Metric < best.Metric) {
				bestBits = prefixLen
				best = r
			}
		}
	}
	return best
}
