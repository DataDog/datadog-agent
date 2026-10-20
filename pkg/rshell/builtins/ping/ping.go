// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ping implements the ping builtin command.
//
// ping — send ICMP echo requests to a network host
//
// Usage: ping [OPTION]... HOST
//
// Sends ICMP echo requests to HOST and reports round-trip statistics.
// Uses github.com/prometheus-community/pro-bing for ICMP operations.
//
// This command always attempts unprivileged mode first (SetPrivileged(false),
// UDP-based ICMP) and automatically falls back to privileged raw-socket mode
// (SetPrivileged(true), SOCK_RAW) if the OS returns a permission error.
//
// # Platform compatibility
//
// Linux:
//
//	Unprivileged ICMP (SOCK_DGRAM) requires the process GID to fall within the
//	kernel sysctl net.ipv4.ping_group_range (default 1–0, i.e. disabled on
//	many distributions). When that range excludes the GID, the kernel returns
//	EPROTONOSUPPORT and the command retries with a raw socket (SOCK_RAW), which
//	requires root or CAP_NET_RAW. CAP_NET_RAW is the preferred deployment
//	approach in containers; alternatively, widen ping_group_range to allow
//	unprivileged operation without any capability.
//
// macOS:
//
//	Unprivileged ICMP (SOCK_DGRAM) is permitted by default for all users on
//	macOS 10.15+. The privileged fallback (SOCK_RAW) requires root. In normal
//	operation the fallback is never needed on macOS.
//
// Windows:
//
//	Both SOCK_DGRAM and SOCK_RAW ICMP sockets on Windows require the process to
//	run as Administrator. Standard user processes receive WSAEACCES ("access is
//	denied") or WSAEPROTONOSUPPORT (10043) when creating the socket. The command
//	attempts the unprivileged path first and retries with privileged mode, but
//	both attempts will fail for non-elevated processes. Run the shell as
//	Administrator (or grant SeNetworkLogonRight) to enable ping on Windows.
//
// Accepted flags:
//
//	-c, --count N
//	    Number of ICMP echo requests to send (default 4, clamped to 1–20).
//
//	-W, --wait DURATION
//	    Time to wait for each reply (default 1s, clamped to 100ms–30s).
//
//	-i, --interval DURATION
//	    Interval between sending packets (default 1s, clamped to 200ms–60s).
//
//	-q, --quiet
//	    Quiet output: suppress per-packet lines; print only statistics.
//
//	-4
//	    Use IPv4 only.
//
//	-6
//	    Use IPv6 only.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Dangerous flags NOT implemented (rejected by pflag as unknown):
//
//	-f           Flood ping — sends packets as fast as possible (DoS vector).
//	-b           Allow pinging broadcast addresses (network DoS vector).
//	-s SIZE      Set packet payload size (not needed; default size is used).
//	-I IFACE     Bind to specific network interface.
//	-p PATTERN   Fill packet with pattern.
//	-R           Record route.
//
// Exit codes:
//
//	0  At least one ICMP echo reply was received.
//	1  No replies received, or the host was unreachable, or bad arguments.
//
// Output format:
//
//	PING host (ip): N data bytes
//	N bytes from ip: icmp_seq=S ttl=T time=R ms
//	...
//	--- host ping statistics ---
//	S packets transmitted, R received, X% packet loss
//	round-trip min/avg/max/stddev = min/avg/max/stddev ms
package ping

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
	"syscall"
	"time"

	probing "github.com/prometheus-community/pro-bing"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

const (
	defaultCount    = 4
	minCount        = 1
	maxCount        = 20
	defaultInterval = time.Second
	minInterval     = 200 * time.Millisecond
	maxInterval     = 60 * time.Second
	defaultWait     = time.Second
	minWait         = 100 * time.Millisecond
	maxWait         = 30 * time.Second
	// icmpPayloadSize matches the POSIX standard ping payload (56 data bytes).
	icmpPayloadSize = 56
	// pingGracePeriod is added to the total deadline to allow the last reply
	// to arrive after the final probe is sent.
	pingGracePeriod = 5 * time.Second
	// maxTotalTimeout is the hard cap on the total wall-clock run time.
	maxTotalTimeout = 120 * time.Second
)

// Cmd is the ping builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "ping",
	Description: "send ICMP echo requests to a network host",
	MakeFlags:   registerFlags,
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit 0")
	count := fs.IntP("count", "c", defaultCount, fmt.Sprintf("number of ICMP packets to send (%d–%d)", minCount, maxCount))
	// StringP instead of DurationP so we accept both Go duration literals
	// (e.g. "1s", "500ms") and the integer/float seconds that iputils ping
	// accepts (e.g. "-W 1", "-i 0.2"). parsePingDuration handles both forms.
	waitStr := fs.StringP("wait", "W", defaultWait.String(), fmt.Sprintf("time to wait for each reply (%v–%v)", minWait, maxWait))
	intervalStr := fs.StringP("interval", "i", defaultInterval.String(), fmt.Sprintf("interval between packets (%v–%v)", minInterval, maxInterval))
	quiet := fs.BoolP("quiet", "q", false, "quiet output: suppress per-packet lines")
	ipv4 := fs.BoolP("ipv4", "4", false, "use IPv4")
	ipv6 := fs.BoolP("ipv6", "6", false, "use IPv6")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		if len(args) == 0 {
			callCtx.Errf("ping: missing host operand\nTry 'ping --help' for more information.\n")
			return builtins.Result{Code: 1}
		}
		if len(args) > 1 {
			callCtx.Errf("ping: too many arguments\nTry 'ping --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		// Parse -W and -i: accept Go duration literals ("1s") and integer/float
		// seconds ("1", "0.2") for iputils ping compatibility.
		wait, err := parsePingDuration(*waitStr)
		if err != nil {
			callCtx.Errf("ping: invalid argument %q for \"-W, --wait\" flag: %v\n", *waitStr, err)
			return builtins.Result{Code: 1}
		}
		interval, err := parsePingDuration(*intervalStr)
		if err != nil {
			callCtx.Errf("ping: invalid argument %q for \"-i, --interval\" flag: %v\n", *intervalStr, err)
			return builtins.Result{Code: 1}
		}

		// Clamp inputs to safe ranges; warn when the user-supplied value
		// is outside the allowed range so the caller is not confused by
		// unexpected behaviour (mirrors find's -maxdepth clamping).
		c := clampInt(*count, minCount, maxCount)
		if *count != c {
			callCtx.Errf("ping: warning: -c %d out of range [%d-%d]; clamped to %d\n", *count, minCount, maxCount, c)
		}
		w := clampDuration(wait, minWait, maxWait)
		if wait != w {
			callCtx.Errf("ping: warning: -W %v out of range [%v-%v]; clamped to %v\n", wait, minWait, maxWait, w)
		}
		iv := clampDuration(interval, minInterval, maxInterval)
		if interval != iv {
			callCtx.Errf("ping: warning: -i %v out of range [%v-%v]; clamped to %v\n", interval, minInterval, maxInterval, iv)
		}

		// Hard total deadline: last-packet deadline + grace period.
		// pro-bing's Timeout is a global wall-clock deadline. The last packet
		// is sent at (count-1)*interval after start; we then wait up to one
		// more 'wait' for its reply. So the total is (count-1)*interval + wait.
		// At clamped minimums (count=1, interval=200ms, wait=100ms) the floor
		// is 0 + 100ms + 5s = 5.1s; callers always get at least that long.
		total := time.Duration(c-1)*iv + w + pingGracePeriod
		if total > maxTotalTimeout {
			total = maxTotalTimeout
			callCtx.Errf("ping: warning: total run time capped at %gs; some probes may not complete\n", maxTotalTimeout.Seconds())
		}
		runCtx, cancel := context.WithTimeout(ctx, total)
		defer cancel()

		return execPing(runCtx, callCtx, args[0], c, w, iv, *quiet, *ipv4, *ipv6)
	}
}

// execPing resolves the host, sets up ICMP probing, and prints results.
func execPing(ctx context.Context, callCtx *builtins.CallContext, host string, count int, wait, interval time.Duration, quiet, ipv4, ipv6 bool) builtins.Result {
	pinger, err := buildPinger(ctx, host, count, wait, interval, ipv4, ipv6)
	if err != nil {
		callCtx.Errf("ping: %v\n", err)
		return builtins.Result{Code: 1}
	}

	// Print the header before opening the socket, matching real ping behaviour.
	// If RunWithContext later fails for a non-permission reason, the header on
	// stdout and the error on stderr is intentional — it mirrors what POSIX ping
	// does and all ping scenarios are marked skip_assert_against_bash: true.
	// pinger.Size is the ICMP echo body size (56 bytes); POSIX "data bytes" refers
	// to this same field.  Pro-bing stores its timestamp and UUID within those
	// 56 bytes, so the displayed count matches the on-wire ICMP payload size.
	// Use host (the original argument) for display; pinger.Addr() returns the
	// numeric IP because buildPinger passes a resolved IP to probing.NewPinger.
	callCtx.Outf("PING %s (%s): %d data bytes\n", host, pinger.IPAddr(), pinger.Size)

	onRecv := makeOnRecv(callCtx, quiet)
	pinger.OnRecv = onRecv

	// Attempt unprivileged mode first.
	pinger.SetPrivileged(false)
	err = pinger.RunWithContext(ctx)

	if err != nil && isPermissionErr(err) {
		// EPERM / EACCES / EPROTONOSUPPORT are returned by the internal
		// p.listen() call before any packet is sent, so pinger.Statistics()
		// is all zeros here and the header printed above is still valid.
		// Retry with raw socket privileges. Pass the already-resolved IP so that
		// buildPinger skips the DNS round-trip and returns immediately.
		// pinger.IPAddr().String() uses net.IPAddr.String(), which preserves the
		// zone identifier for link-local IPv6 addresses (e.g. "fe80::1%eth0"),
		// ensuring the OS can route the retry correctly.
		p2, err2 := buildPinger(ctx, pinger.IPAddr().String(), count, wait, interval, ipv4, ipv6)
		if err2 != nil {
			callCtx.Errf("ping: %v\n", err2)
			return builtins.Result{Code: 1}
		}
		p2.OnRecv = onRecv
		p2.SetPrivileged(true)
		err = p2.RunWithContext(ctx)
		pinger = p2
	}

	// Print statistics unconditionally — even on non-permission errors and
	// context cancellation. This mirrors POSIX ping which always prints
	// partial statistics (with 0 received) before exiting, whether due to
	// SIGINT, network-unreachable, or timeout.
	stats := pinger.Statistics()
	printStats(callCtx, host, stats)

	if err != nil && ctx.Err() == nil {
		callCtx.Errf("ping: %v\n", err)
		return builtins.Result{Code: 1}
	}

	if stats.PacketsRecv == 0 {
		return builtins.Result{Code: 1}
	}
	return builtins.Result{}
}

// buildPinger creates and configures a Pinger with the given parameters.
// DNS resolution is context-aware: cancellation propagates into the DNS query
// itself, avoiding goroutine leaks that would result from wrapping the
// non-context net.ResolveIPAddr.
func buildPinger(ctx context.Context, host string, count int, wait, interval time.Duration, ipv4, ipv6 bool) (*probing.Pinger, error) {
	if ipv4 && ipv6 {
		return nil, fmt.Errorf("-4 and -6 are mutually exclusive")
	}

	// When a family flag is given and the host is a hostname (not a numeric
	// IP literal), use a family-specific lookup so that we only wait for the
	// requested record type (A or AAAA).  This avoids unnecessary latency
	// when, for example, -4 is requested but AAAA records are slow or broken.
	// LookupIP returns []net.IP without zone info, which is acceptable here
	// because DNS-resolved IPv6 link-local addresses (the only case where zone
	// matters) are extremely rare in practice.
	// For numeric IP literals, use LookupIPAddr instead: parsing is instant
	// (no DNS query is issued), preserves our custom "no ip6/ip4 address" error
	// messages when the literal doesn't match the requested family, and (unlike
	// LookupIP) preserves the zone identifier for scoped IP literals such as
	// "fe80::1%eth0". net.ParseIP returns nil for scoped addresses, so we also
	// detect them via isScopedIPLiteral.
	// When no flag is given, use LookupIPAddr (dual-stack) and select below.
	isNumericIP := net.ParseIP(host) != nil || isScopedIPLiteral(host)
	var addrs []net.IPAddr
	switch {
	case ipv4 && !isNumericIP:
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			addrs = append(addrs, net.IPAddr{IP: ip})
		}
	case ipv6 && !isNumericIP:
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip6", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			addrs = append(addrs, net.IPAddr{IP: ip})
		}
	default:
		var err error
		addrs, err = net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, err
		}
	}

	// Select an address from the resolved set.
	// When -4/-6 is given for a hostname: addrs already match the family;
	// take the first.  For numeric IP literals (resolved via LookupIPAddr)
	// we must still filter so that e.g. "ping -4 ::1" correctly errors.
	// When neither flag is given: prefer IPv4 (traditional ping default) so
	// that AAAA-first DNS results on hosts without working IPv6 do not cause
	// spurious failures; fall back to the first IPv6 address if no IPv4 found.
	// Known limitation: IPv4-mapped IPv6 addresses (e.g. ::ffff:127.0.0.1)
	// are indistinguishable from native IPv4 at the net.IP byte level — Go's
	// net.ParseIP collapses both to the same 16-byte representation, and
	// To4() returns non-nil for both.  As a result, "ping -6 ::ffff:x.x.x.x"
	// behaves identically to "ping -6 x.x.x.x" and returns "no ip6 address".
	// This is not fixable without tracking the original string form, and DNS
	// AAAA records never return IPv4-mapped addresses in practice.
	var resolved *net.IPAddr
	if ipv4 || ipv6 {
		for i := range addrs {
			a := &addrs[i]
			isV4 := a.IP.To4() != nil
			if (ipv4 && isV4) || (ipv6 && !isV4) {
				resolved = a
				break
			}
		}
	} else {
		// No family flag: prefer IPv4, fall back to first IPv6.
		var ipv6Fallback *net.IPAddr
		for i := range addrs {
			a := &addrs[i]
			if a.IP.To4() != nil {
				resolved = a
				break
			}
			if ipv6Fallback == nil {
				ipv6Fallback = a
			}
		}
		if resolved == nil {
			resolved = ipv6Fallback
		}
	}
	if resolved == nil {
		family := "ip4"
		if ipv6 {
			family = "ip6"
		}
		return nil, fmt.Errorf("no %s address for host %q", family, host)
	}

	// Reject broadcast and multicast destinations to prevent unintended DoS.
	// The -f and -b flags are already rejected by the flag parser; this
	// catches cases where the resolved IP itself is a broadcast or multicast addr.
	//
	// NOTE: pro-bing v0.8.0 automatically retries sends with SO_BROADCAST set
	// on Linux when WriteTo returns EACCES (ping.go sendICMP loop). This means
	// that even without -b, a directed-broadcast address (e.g. 10.0.0.255) would
	// result in actual ICMP broadcast traffic being sent. We therefore reject
	// any IPv4 address whose last octet is 255, which covers:
	//   - The limited broadcast: 255.255.255.255
	//   - All subnet-directed broadcasts on standard /8, /16, /24 networks
	//     (whose broadcast address always ends in 255).
	// Known limitation: directed broadcasts on non-standard subnets (e.g. a /25
	// network whose broadcast is x.x.x.127) are NOT blocked here. Without the
	// subnet mask, we cannot enumerate all possible broadcast addresses; blocking
	// all addresses with last octet ≤ 127 would be far too aggressive. In those
	// environments the OS still enforces SO_BROADCAST for raw sockets except
	// that pro-bing's auto-retry circumvents it on Linux. Additionally, if
	// SetBroadcastFlag itself fails in the pro-bing sendICMP loop, the loop has
	// no exit path until the 120 s context deadline fires — so on such subnets
	// the command may stall rather than fail immediately. This is an upstream
	// pro-bing v0.8.0 limitation; the 120 s hard cap provides the safety bound.
	// Known false-positive: on subnets wider than /24 (e.g. /16 or /23),
	// the last octet can be 255 on a valid unicast host (e.g. 10.0.1.255 on
	// 10.0.0.0/16, whose broadcast is 10.0.255.255). These rare addresses are
	// blocked by this heuristic. Without the subnet mask the shell cannot
	// distinguish them from broadcast addresses, and the safety trade-off
	// (block a rare unicast > risk unintended broadcast) is appropriate for a
	// restricted shell environment.
	ip := resolved.IP
	if ip.IsUnspecified() {
		return nil, fmt.Errorf("unspecified destination not allowed: %s", ip)
	}
	if ip.IsMulticast() {
		return nil, fmt.Errorf("multicast destination not allowed: %s", ip)
	}
	// Block limited broadcast and subnet-directed broadcast addresses.
	if ip4 := ip.To4(); ip4 != nil && ip4[3] == 255 {
		return nil, fmt.Errorf("broadcast destination not allowed: %s", ip)
	}

	// Pass the numeric IP; pro-bing's internal net.ResolveIPAddr returns
	// immediately for a numeric address, so no second DNS round-trip occurs.
	// NOTE: NewPinger calls net.ResolveIPAddr without a context, but since
	// we always pass a numeric IP here, that call is synchronous and instant —
	// no goroutine leak or context-cancellation gap exists in practice.
	p, err := probing.NewPinger(resolved.String())
	if err != nil {
		return nil, err
	}
	p.Count = count
	p.Size = icmpPayloadSize
	// pro-bing Timeout is a global wall-clock deadline, not per-packet.
	// The last probe is sent at (count-1)*interval; we then wait up to
	// one 'wait' for its reply.
	p.Timeout = time.Duration(count-1)*interval + wait
	p.Interval = interval
	p.SetLogger(probing.NoopLogger{})
	if ipv4 {
		p.SetNetwork("ip4")
	} else if ipv6 {
		p.SetNetwork("ip6")
	}
	return p, nil
}

// makeOnRecv returns the OnRecv callback. In quiet mode it returns nil so
// no per-packet output is written.
func makeOnRecv(callCtx *builtins.CallContext, quiet bool) func(*probing.Packet) {
	if quiet {
		return nil
	}
	return func(pkt *probing.Packet) {
		// pro-bing sequences start at 0; add 1 to match POSIX/bash ping convention
		// where the first reply carries icmp_seq=1.
		callCtx.Outf("%d bytes from %s: icmp_seq=%d ttl=%d time=%.3f ms\n",
			pkt.Nbytes, pkt.IPAddr, pkt.Seq+1, pkt.TTL, durToMS(pkt.Rtt))
	}
}

// printStats writes the two summary lines that every ping run ends with.
// host is the original argument (hostname or IP) for display in the footer.
func printStats(callCtx *builtins.CallContext, host string, stats *probing.Statistics) {
	callCtx.Outf("\n--- %s ping statistics ---\n", host)
	callCtx.Outf("%d packets transmitted, %d received, %.1f%% packet loss\n",
		stats.PacketsSent, stats.PacketsRecv, stats.PacketLoss)
	if stats.PacketsRecv > 0 {
		callCtx.Outf("round-trip min/avg/max/stddev = %.3f/%.3f/%.3f/%.3f ms\n",
			durToMS(stats.MinRtt), durToMS(stats.AvgRtt),
			durToMS(stats.MaxRtt), durToMS(stats.StdDevRtt))
	}
}

// printHelp writes the usage text to stdout.
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: ping [OPTION]... HOST\n")
	callCtx.Out("Send ICMP echo requests to HOST and report statistics.\n\n")
	callCtx.Out("Options:\n")
	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
	callCtx.Out("\nNote: the following flags are not supported for safety and will be rejected:\n")
	callCtx.Out("  -f (flood), -b (broadcast), -s (packet size), -I (interface), -p (pattern), -R (record route)\n")
}

// clampInt returns v clamped to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampDuration returns v clamped to [lo, hi].
func clampDuration(v, lo, hi time.Duration) time.Duration {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// durToMS converts a duration to milliseconds as a float64.
func durToMS(d time.Duration) float64 {
	return float64(d.Microseconds()) / 1000.0
}

// parsePingDuration parses a duration string for the -W and -i flags.
// It accepts Go duration literals (e.g. "1s", "500ms") and the plain
// integer/float seconds used by iputils ping (e.g. "1", "0.2", "1.5").
// Negative values are rejected regardless of form; the caller's
// clampDuration handles out-of-range positive values.
func parsePingDuration(s string) (time.Duration, error) {
	// Try Go duration literal first — fastest and most precise.
	if d, err := time.ParseDuration(s); err == nil {
		if d < 0 {
			return 0, fmt.Errorf("negative duration %q not allowed", s)
		}
		return d, nil
	}
	// Fall back to plain numeric seconds (integer or float) as iputils does.
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration %q: must be a Go duration (e.g. \"1s\", \"500ms\") or seconds (e.g. \"1\", \"0.5\")", s)
	}
	// Reject non-finite values and values that would overflow time.Duration
	// (int64 nanoseconds; max ~9.2 billion seconds = ~292 years).
	const maxDurationSec = float64(math.MaxInt64 / int64(time.Second))
	if math.IsNaN(f) || math.IsInf(f, 0) || f > maxDurationSec {
		return 0, fmt.Errorf("invalid duration %q: must be a finite positive number", s)
	}
	if f < 0 {
		return 0, fmt.Errorf("negative duration %q not allowed", s)
	}
	return time.Duration(f * float64(time.Second)), nil
}

// isPermissionErr reports whether err indicates that the process lacks the
// privilege to open a raw ICMP socket. When true, the caller should retry
// with privileged raw-socket mode.
//
// This function is only called on errors returned by pinger.RunWithContext,
// which come from the ICMP socket layer — not from DNS. DNS errors are caught
// earlier in buildPinger and returned to the caller before RunWithContext is
// ever invoked, so "permission denied" strings here always originate from
// socket creation, never from a DNS resolver response.
//
// We detect three classes of failure:
//  1. EPERM / EACCES — classic Unix permission denials.
//  2. EPROTONOSUPPORT — returned on Linux when the kernel's
//     net.ipv4.ping_group_range does not cover the process GID and the
//     unprivileged UDP-based ICMP path is unavailable; privileged raw
//     sockets are unaffected and should be tried.
//  3. String-based fallback for Windows and platforms that wrap errors.
func isPermissionErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) ||
		errors.Is(err, syscall.EPROTONOSUPPORT) {
		return true
	}
	// String-based fallback for Windows and platforms where pro-bing wraps
	// the syscall error in a way that breaks errors.Is unwrapping.  On Unix,
	// these strings overlap with the errors.Is checks above (e.g.
	// syscall.EACCES produces "permission denied"), but they are kept for
	// defence-in-depth: pro-bing's internal error path may change across
	// versions.  The overlap is harmless — a match that is already caught
	// above short-circuits before reaching this block.
	//
	// NOTE: "operation not permitted" / "permission denied" / "protocol not
	// supported" cannot originate from DNS here because DNS errors are caught
	// in buildPinger before RunWithContext is ever called (see the function
	// comment above).  Every error reaching this point comes from the ICMP
	// socket layer.
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "operation not permitted") ||
		strings.Contains(msg, "access is denied") ||
		strings.Contains(msg, "permission denied") ||
		strings.Contains(msg, "protocol not supported") ||
		// Windows: WSAEPROTONOSUPPORT (10043) — returned by pro-bing when an
		// unprivileged raw socket cannot be created; privileged mode should be tried.
		strings.Contains(msg, "the requested protocol has not been configured")
}

// isScopedIPLiteral reports whether host is a scoped IP address literal with a
// zone suffix, such as "fe80::1%eth0" or "192.168.1.1%eth0". net.ParseIP
// rejects the '%' zone separator, so this function strips the zone and parses
// the base address. In practice only scoped IPv6 link-local addresses carry a
// zone, but the function matches any IP+zone for correctness.
func isScopedIPLiteral(host string) bool {
	idx := strings.IndexByte(host, '%')
	if idx < 0 {
		return false
	}
	return net.ParseIP(host[:idx]) != nil
}
