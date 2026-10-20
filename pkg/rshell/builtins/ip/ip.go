// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ip implements the ip builtin command.
//
// ip — show network interfaces, addresses, and routing
//
// Usage: ip [GLOBAL-OPTIONS] OBJECT [COMMAND [ARGUMENTS]]
//
// Query network interface and routing information. Only read-only subcommands
// are supported. All write operations (add, del, flush, change, replace, set)
// and dangerous execution vectors (netns exec, -batch, -force) are rejected
// with exit code 1.
//
// GLOBAL OPTIONS
//
//	-o, --oneline
//	    Output each record on a single line; internal newlines are represented
//	    by a backslash followed by the continuation content (matching real ip
//	    -o format). Useful for machine parsing by AI agents.
//
//	--brief
//	    Print a compact tabular summary: interface name, state, and addresses
//	    only. Mutually compatible with -4/-6. (Note: the real ip command uses
//	    -br as a shorthand; our builtin uses --brief instead.)
//
//	-4
//	    Restrict output to IPv4 only (for addr/link; route always uses IPv4).
//
//	-6
//	    Restrict address output to IPv6 only. Not supported for route.
//
//	-h, --help
//	    Print this usage message to stdout and exit 0.
//
// OBJECTS AND COMMANDS
//
//	addr [show] [dev IFNAME]
//	    Show IP addresses assigned to all network interfaces, or to the
//	    single interface named IFNAME when "dev IFNAME" is given.
//	    "show" is the default command when no command is specified.
//
//	link [show] [dev IFNAME]
//	    Show link-layer information (MTU, hardware address, flags) for all
//	    interfaces, or for the single interface named IFNAME.
//	    "show" is the default command when no command is specified.
//
//	route [show|list]
//	    Show the IPv4 routing table, read from /proc/net/route.
//	    Only supported on Linux; returns an error on other platforms.
//
//	route get ADDRESS
//	    Show the route that would be used to reach ADDRESS, selected by
//	    longest-prefix-match over the IPv4 routing table.
//	    Only supported on Linux; returns an error on other platforms.
//
// BLOCKED FLAGS AND SUBCOMMANDS (exit 1 with an explanatory error)
//
//	-b, -B, -batch      Reads ip commands from FILE — arbitrary command
//	                    execution vector (GTFOBins).
//	-force              Suppresses errors; companion to -batch (GTFOBins).
//	-n, --netns         Switches network namespace — privilege escalation.
//	ip netns            Network namespace management — shell escape via
//	                    "ip netns exec <ns> <cmd>".
//	addr add/del/flush/change/replace    Write operations (blocked).
//	link set/add/del/change              Write operations (blocked).
//	route add/del/delete/change/replace  Write operations (blocked).
//	route flush/save/restore             Write operations (blocked).
//
// Exit codes:
//
//	0  Query completed successfully.
//	1  Unknown subcommand, unsupported flag, write operation attempted,
//	   unsupported platform (route), or the named interface does not exist.
//
// Network access:
//
//	addr and link use Go's net.Interfaces() for read-only enumeration of OS
//	network interfaces and their addresses; the AllowedPaths sandbox is not
//	involved. route reads /proc/net/route via builtins/internal/procnetroute using
//	os.Open directly (Linux only); the AllowedPaths sandbox is not involved.
//
// Memory safety for route:
//
//	/proc/net/route is read line-by-line with a per-line cap of MaxLineBytes
//	(1 MiB). At most MaxRoutes (10 000) entries are loaded. All read loops
//	check ctx.Err() at each iteration to honour the execution timeout.
//
// Output differences from real ip:
//
//	The qdisc field is omitted from interface header lines. For route show/list,
//	the proto/scope/src fields are not included (not available from
//	/proc/net/route alone). For route get, the src, uid, and cache fields
//	present in real ip-route(8) output are also omitted (not derivable from
//	/proc/net/route alone).
package ip

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/internal/procnetroute"
)

// ProcNetRoutePath is the proc filesystem root used to locate the routing table.
// ReadRoutes opens ProcNetRoutePath/net/route.
//
// Concurrency contract: this variable is written only in tests (via the
// writeProcNetRoute helper) and is never mutated by production code after
// package initialization. Production callers therefore need no lock to read it.
// Test code that mutates ProcNetRoutePath must hold procNetRouteMu (defined in
// ip_linux_test.go) for the duration of the test to serialise test mutations
// and prevent races between concurrent test goroutines.
var ProcNetRoutePath = procnetroute.DefaultProcPath

// MaxLineBytes re-exports the procnetroute constant for test access.
const MaxLineBytes = procnetroute.MaxLineBytes

// Cmd is the ip builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "ip",
	Description: "show network interface and routing information",
	MakeFlags:   registerFlags,
}

// displayOpts holds the resolved global display options.
type displayOpts struct {
	oneline bool
	brief   bool
	ipv4    bool
	ipv6    bool
}

func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	oneline := fs.BoolP("oneline", "o", false, "output each record on a single line")
	brief := fs.Bool("brief", false, "print brief information in tabular format")
	ipv4 := fs.BoolP("ipv4", "4", false, "show only IPv4 addresses")
	ipv6 := fs.BoolP("ipv6", "6", false, "show only IPv6 addresses")

	return func(ctx context.Context, callCtx *builtins.CallContext, args []string) builtins.Result {
		if *help {
			printHelp(callCtx, fs)
			return builtins.Result{}
		}

		do := displayOpts{
			oneline: *oneline,
			brief:   *brief,
			ipv4:    *ipv4,
			ipv6:    *ipv6,
		}

		// If both -4 and -6 are given, neither filter applies.
		if do.ipv4 && do.ipv6 {
			do.ipv4 = false
			do.ipv6 = false
		}

		if len(args) == 0 {
			callCtx.Errf("ip: object required\nTry 'ip --help' for more information.\n")
			return builtins.Result{Code: 1}
		}

		object, rest := args[0], args[1:]

		switch object {
		case "addr", "address":
			return runAddr(ctx, callCtx, do, rest)
		case "link":
			return runLink(ctx, callCtx, do, rest)
		case "route":
			return routeCmd(ctx, callCtx, do, rest)
		case "netns":
			callCtx.Errf("ip: 'netns' subcommand is blocked (shell escape vector via 'ip netns exec')\n")
			return builtins.Result{Code: 1}
		default:
			callCtx.Errf("ip: object %q is not supported\nSupported objects: addr, link, route\n", object)
			return builtins.Result{Code: 1}
		}
	}
}

// printHelp writes the usage text to stdout.
func printHelp(callCtx *builtins.CallContext, fs *builtins.FlagSet) {
	callCtx.Out("Usage: ip [GLOBAL-OPTIONS] OBJECT [COMMAND [ARGUMENTS]]\n")
	callCtx.Out("Show network interface and routing information.\n\n")
	callCtx.Out("Supported objects:\n")
	callCtx.Out("  addr [show] [dev IFNAME]  Show IP addresses\n")
	callCtx.Out("  link [show] [dev IFNAME]  Show link-layer information\n")
	callCtx.Out("  route [show|list]         Show IPv4 routing table (Linux only)\n")
	callCtx.Out("  route get ADDRESS         Show route to ADDRESS (Linux only)\n\n")
	callCtx.Out("Global options:\n")
	fs.SetOutput(callCtx.Stdout)
	fs.PrintDefaults()
	callCtx.Out("\nNote: -b/-B/-batch, -force, -n/--netns, and 'ip netns' are blocked for safety.\n")
	callCtx.Out("Note: the real ip command's -br flag is --brief in this builtin.\n")
}

// parseShowArgs parses the argument list after an object name ("addr" or "link").
// Returns (devFilter, error). devFilter="" means show all interfaces.
// Write subcommands return a descriptive error.
func parseShowArgs(object string, args []string) (string, error) {
	if len(args) == 0 {
		return "", nil
	}

	// Consume optional "show"/"list"/"lst" command keyword.
	switch args[0] {
	case "show", "list", "lst":
		args = args[1:]
	case "add", "append", "replace", "del", "delete", "flush", "set", "change":
		return "", fmt.Errorf("'ip %s %s' is not supported (write operations are blocked for safety)", object, args[0])
	}

	// Parse optional "dev IFNAME" filter; any other token is an error.
	var devFilter string
	for len(args) > 0 {
		switch args[0] {
		case "dev":
			if len(args) < 2 {
				return "", fmt.Errorf("'dev' requires an interface name argument")
			}
			devFilter = args[1]
			args = args[2:]
		default:
			return "", fmt.Errorf("unknown token %q after 'ip %s show'", args[0], object)
		}
	}
	return devFilter, nil
}

// getInterfaces returns all interfaces, optionally filtered by devName.
// If devName is set and no matching interface is found, returns an error.
func getInterfaces(devName string) ([]net.Interface, error) {
	all, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("cannot enumerate interfaces: %w", err)
	}
	if devName == "" {
		return all, nil
	}
	for _, iface := range all {
		if iface.Name == devName {
			return []net.Interface{iface}, nil
		}
	}
	return nil, fmt.Errorf("cannot find device %q", devName)
}

// flagsStr returns the kernel flags string, e.g. "<BROADCAST,MULTICAST,UP,LOWER_UP>".
func flagsStr(flags net.Flags) string {
	var parts []string
	if flags&net.FlagBroadcast != 0 {
		parts = append(parts, "BROADCAST")
	}
	if flags&net.FlagMulticast != 0 {
		parts = append(parts, "MULTICAST")
	}
	if flags&net.FlagLoopback != 0 {
		parts = append(parts, "LOOPBACK")
	}
	if flags&net.FlagPointToPoint != 0 {
		parts = append(parts, "POINTOPOINT")
	}
	if flags&net.FlagUp != 0 {
		parts = append(parts, "UP")
	}
	if flags&net.FlagRunning != 0 {
		parts = append(parts, "LOWER_UP")
	}
	return "<" + strings.Join(parts, ",") + ">"
}

// ifaceState derives the ip-style state string from interface flags.
func ifaceState(flags net.Flags) string {
	if flags&net.FlagLoopback != 0 {
		// Loopback interfaces lack a physical carrier, so ip reports UNKNOWN.
		return "UNKNOWN"
	}
	up := flags&net.FlagUp != 0
	running := flags&net.FlagRunning != 0
	switch {
	case up && running:
		return "UP"
	case !up:
		return "DOWN"
	default:
		return "UNKNOWN"
	}
}

// ifaceLinkType returns the "link/TYPE" label for an interface.
func ifaceLinkType(iface net.Interface) string {
	switch {
	case iface.Flags&net.FlagLoopback != 0:
		return "loopback"
	case len(iface.HardwareAddr) == 6:
		return "ether"
	case iface.Flags&net.FlagPointToPoint != 0:
		return "ppp"
	default:
		return "none"
	}
}

// ifaceMAC returns the interface MAC address as a colon-hex string.
// Returns "00:00:00:00:00:00" when the hardware address is absent.
func ifaceMAC(iface net.Interface) string {
	s := iface.HardwareAddr.String()
	if s == "" {
		return "00:00:00:00:00:00"
	}
	return s
}

// ifaceBrdMAC returns the broadcast MAC for an interface, or "" if the
// interface type does not use a broadcast address.
func ifaceBrdMAC(iface net.Interface) string {
	switch {
	case iface.Flags&net.FlagLoopback != 0:
		return "00:00:00:00:00:00"
	case iface.Flags&net.FlagBroadcast != 0:
		return "ff:ff:ff:ff:ff:ff"
	default:
		return ""
	}
}

// ipv4Broadcast computes the IPv4 broadcast address for ipNet.
// Returns "" for IPv6 networks.
func ipv4Broadcast(ipNet *net.IPNet) string {
	ip4 := ipNet.IP.To4()
	if ip4 == nil {
		return ""
	}
	mask := ipNet.Mask
	brd := make(net.IP, 4)
	for i := range 4 {
		brd[i] = ip4[i] | ^mask[i]
	}
	return brd.String()
}

// addrScope returns the RTSCOPE name for an IP address.
func addrScope(ip net.IP) string {
	if ip.IsLoopback() {
		return "host"
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return "link"
	}
	return "global"
}

// runAddr implements "ip addr [show] [dev IFNAME]".
func runAddr(ctx context.Context, callCtx *builtins.CallContext, do displayOpts, args []string) builtins.Result {
	devFilter, err := parseShowArgs("addr", args)
	if err != nil {
		callCtx.Errf("ip: %s\n", err)
		return builtins.Result{Code: 1}
	}

	ifaces, err := getInterfaces(devFilter)
	if err != nil {
		callCtx.Errf("ip: %s\n", err)
		return builtins.Result{Code: 1}
	}

	for _, iface := range ifaces {
		if ctx.Err() != nil {
			break
		}
		if err := printAddrEntry(callCtx, do, iface); err != nil {
			callCtx.Errf("ip: %s\n", err)
			return builtins.Result{Code: 1}
		}
	}
	return builtins.Result{}
}

// runLink implements "ip link [show] [dev IFNAME]".
func runLink(ctx context.Context, callCtx *builtins.CallContext, do displayOpts, args []string) builtins.Result {
	devFilter, err := parseShowArgs("link", args)
	if err != nil {
		callCtx.Errf("ip: %s\n", err)
		return builtins.Result{Code: 1}
	}

	ifaces, err := getInterfaces(devFilter)
	if err != nil {
		callCtx.Errf("ip: %s\n", err)
		return builtins.Result{Code: 1}
	}

	for _, iface := range ifaces {
		if ctx.Err() != nil {
			break
		}
		printLinkEntry(callCtx, do, iface)
	}
	return builtins.Result{}
}

// printAddrEntry renders one interface's address information.
func printAddrEntry(callCtx *builtins.CallContext, do displayOpts, iface net.Interface) error {
	addrs, err := iface.Addrs()
	if err != nil {
		return fmt.Errorf("cannot get addresses for %s: %w", iface.Name, err)
	}

	state := ifaceState(iface.Flags)
	mac := ifaceMAC(iface)
	brdMAC := ifaceBrdMAC(iface)
	ltype := ifaceLinkType(iface)
	flags := flagsStr(iface.Flags)

	if do.brief {
		var addrParts []string
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if do.ipv4 && ip.To4() == nil {
				continue
			}
			if do.ipv6 && ip.To4() != nil {
				continue
			}
			addrParts = append(addrParts, ipNet.String())
		}
		addrStr := strings.Join(addrParts, " ")
		if addrStr != "" {
			addrStr = " " + addrStr
		}
		callCtx.Outf("%-16s %-13s%s\n", iface.Name, state, addrStr)
		return nil
	}

	if do.oneline {
		// Oneline mode: one output line per address (no interface header line).
		prefix := fmt.Sprintf("%d: %s", iface.Index, iface.Name)
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP
			if do.ipv4 && ip.To4() == nil {
				continue
			}
			if do.ipv6 && ip.To4() != nil {
				continue
			}
			var addrLine string
			if ip.To4() != nil {
				brd := ipv4Broadcast(ipNet)
				if brd != "" && iface.Flags&net.FlagBroadcast != 0 {
					addrLine = fmt.Sprintf("    inet %s brd %s scope %s %s",
						ipNet.String(), brd, addrScope(ip), iface.Name)
				} else {
					addrLine = fmt.Sprintf("    inet %s scope %s %s",
						ipNet.String(), addrScope(ip), iface.Name)
				}
			} else {
				addrLine = fmt.Sprintf("    inet6 %s scope %s",
					ipNet.String(), addrScope(ip))
			}
			callCtx.Outf("%s%s\\       valid_lft forever preferred_lft forever\n",
				prefix, addrLine)
		}
		return nil
	}

	// Normal multi-line output.
	callCtx.Outf("%d: %s: %s mtu %d state %s group default qlen 1000\n",
		iface.Index, iface.Name, flags, iface.MTU, state)
	if brdMAC != "" {
		callCtx.Outf("    link/%s %s brd %s\n", ltype, mac, brdMAC)
	} else {
		callCtx.Outf("    link/%s %s\n", ltype, mac)
	}
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipNet.IP
		if do.ipv4 && ip.To4() == nil {
			continue
		}
		if do.ipv6 && ip.To4() != nil {
			continue
		}
		scope := addrScope(ip)
		if ip.To4() != nil {
			brd := ipv4Broadcast(ipNet)
			if brd != "" && iface.Flags&net.FlagBroadcast != 0 {
				callCtx.Outf("    inet %s brd %s scope %s %s\n",
					ipNet.String(), brd, scope, iface.Name)
			} else {
				callCtx.Outf("    inet %s scope %s %s\n",
					ipNet.String(), scope, iface.Name)
			}
		} else {
			callCtx.Outf("    inet6 %s scope %s\n", ipNet.String(), scope)
		}
		callCtx.Out("       valid_lft forever preferred_lft forever\n")
	}
	return nil
}

// printLinkEntry renders one interface's link-layer information.
func printLinkEntry(callCtx *builtins.CallContext, do displayOpts, iface net.Interface) {
	state := ifaceState(iface.Flags)
	mac := ifaceMAC(iface)
	brdMAC := ifaceBrdMAC(iface)
	ltype := ifaceLinkType(iface)
	flags := flagsStr(iface.Flags)

	if do.brief {
		callCtx.Outf("%-16s %-13s %s %s\n", iface.Name, state, mac, flags)
		return
	}

	headerLine := fmt.Sprintf(
		"%d: %s: %s mtu %d state %s mode DEFAULT group default qlen 1000",
		iface.Index, iface.Name, flags, iface.MTU, state)

	var linkLine string
	if brdMAC != "" {
		linkLine = fmt.Sprintf("    link/%s %s brd %s", ltype, mac, brdMAC)
	} else {
		linkLine = fmt.Sprintf("    link/%s %s", ltype, mac)
	}

	if do.oneline {
		callCtx.Outf("%s\\%s\n", headerLine, linkLine)
		return
	}

	callCtx.Outf("%s\n%s\n", headerLine, linkLine)
}

// ---------------------------------------------------------------------------
// ip route implementation
// ---------------------------------------------------------------------------

// routeCmd dispatches ip route subcommands.
func routeCmd(ctx context.Context, callCtx *builtins.CallContext, do displayOpts, args []string) builtins.Result {
	if do.ipv6 {
		callCtx.Errf("ip: route: IPv6 routing not supported\n")
		return builtins.Result{Code: 1}
	}

	sub := "show"
	if len(args) > 0 {
		sub = args[0]

		// Validate the subcommand before checking display flags so that an unknown
		// subcommand produces a precise error rather than "flag not supported".
		// Subcommand matching is intentionally case-sensitive (lowercase only),
		// matching real iproute2 behavior — "SHOW" and "Show" are not valid.
		switch sub {
		case "show", "list", "get":
			// valid read subcommands — validated below
		case "add", "del", "delete", "change", "replace", "flush", "save", "restore":
			callCtx.Errf("ip: route: %s: write operations are not permitted\n", sub)
			return builtins.Result{Code: 1}
		default:
			// Match iproute2 exit code (255) and error format for unknown subcommands.
			callCtx.Errf("Command %q is unknown, try \"ip route help\".\n", sub)
			return builtins.Result{Code: 255}
		}
	}

	// --oneline and --brief are not supported for route output regardless of
	// how the subcommand was specified (explicit "show"/"list" or the default).
	if do.oneline || do.brief {
		callCtx.Errf("ip: route: -o/--oneline and --brief flags are not supported for route output\n")
		return builtins.Result{Code: 1}
	}

	switch sub {
	case "show", "list":
		// args[0] is the subcommand ("show"/"list"); args[1] would be the first
		// unsupported argument. When no subcommand was typed ("ip route"), args
		// is empty and sub defaults to "show", so the len(args) > 1 guard cannot
		// panic (args[1] is only accessed when len(args) >= 2).
		if len(args) > 1 {
			callCtx.Errf("ip: route %s: unsupported argument %q\n", sub, args[1])
			return builtins.Result{Code: 1}
		}
		return routeShow(ctx, callCtx)
	case "get":
		if len(args) < 2 {
			callCtx.Errf("ip: route get: missing address argument\n")
			return builtins.Result{Code: 1}
		}
		if len(args) > 2 {
			callCtx.Errf("ip: route get: unsupported argument %q\n", args[2])
			return builtins.Result{Code: 1}
		}
		return routeGet(ctx, callCtx, args[1])
	default:
		// unreachable: the first switch above ensures only "show", "list", "get"
		// reach here, but avoid panic in builtins — return an error instead.
		callCtx.Errf("Command %q is unknown, try \"ip route help\".\n", sub)
		return builtins.Result{Code: 255}
	}
}

// routeShow prints the IPv4 routing table in ip-route(8) format.
func routeShow(ctx context.Context, callCtx *builtins.CallContext) builtins.Result {
	routes, err := procnetroute.ReadRoutes(ctx, ProcNetRoutePath)
	if err != nil {
		callCtx.Errf("ip: route: %s\n", callCtx.PortableErr(err))
		return builtins.Result{Code: 1}
	}

	for i := range routes {
		if ctx.Err() != nil {
			return builtins.Result{}
		}
		callCtx.Outf("%s\n", formatRoute(&routes[i]))
	}
	return builtins.Result{}
}

// routeGet finds and prints the route used to reach addr.
func routeGet(ctx context.Context, callCtx *builtins.CallContext, addr string) builtins.Result {
	addrVal, ok := parseIPv4(addr)
	if !ok {
		callCtx.Errf("ip: route get: invalid address %q\n", addr)
		return builtins.Result{Code: 1}
	}

	routes, err := procnetroute.ReadRoutes(ctx, ProcNetRoutePath)
	if err != nil {
		callCtx.Errf("ip: route get: %s\n", callCtx.PortableErr(err))
		return builtins.Result{Code: 1}
	}

	best := procnetroute.LongestPrefixMatch(routes, addrVal)
	// Split nil vs. reject: nil means no route matched at all; FlagReject means
	// the kernel explicitly blocks this destination. Both produce "network
	// unreachable", but the distinction is preserved for future diagnostics.
	if best == nil {
		callCtx.Errf("ip: route get: network unreachable\n")
		return builtins.Result{Code: 1}
	}
	if best.Flags&procnetroute.FlagReject != 0 {
		callCtx.Errf("ip: route get: network unreachable\n")
		return builtins.Result{Code: 1}
	}

	var b strings.Builder
	b.WriteString(addr)
	if best.Flags&procnetroute.FlagGateway != 0 {
		b.WriteString(" via ")
		b.WriteString(procnetroute.HexToIPStr(best.GW))
	}
	// Reject routes have iface="*"; omit "dev" for them to match real ip-route(8)
	// output. With the FlagReject guard above this branch is unreachable for
	// reject routes today, but the guard makes the invariant explicit.
	if best.Flags&procnetroute.FlagReject == 0 {
		b.WriteString(" dev ")
		b.WriteString(best.Iface)
	}
	if best.Metric != 0 {
		b.WriteString(" metric ")
		b.WriteString(strconv.FormatUint(uint64(best.Metric), 10))
	}
	b.WriteByte('\n')
	callCtx.Out(b.String())
	return builtins.Result{}
}

// formatRoute returns the ip-route(8) display string for r.
func formatRoute(r *procnetroute.Route) string {
	var b strings.Builder

	// Reject (unreachable/blackhole) routes are displayed with a "unreachable"
	// prefix and no "dev" field, matching real ip-route(8) output.
	if r.Flags&procnetroute.FlagReject != 0 {
		b.WriteString("unreachable ")
		if r.Dest == 0 && r.Mask == 0 {
			// The "unreachable default" case (reject route with /0) is theoretically
			// handled but is unlikely to be generated by a real Linux kernel.
			b.WriteString("default")
		} else {
			b.WriteString(procnetroute.HexToIPStr(r.Dest))
			prefixLen := procnetroute.Popcount(r.Mask)
			if prefixLen != 32 {
				b.WriteByte('/')
				b.WriteString(strconv.Itoa(prefixLen))
			}
		}
		if r.Metric != 0 {
			b.WriteString(" metric ")
			b.WriteString(strconv.FormatUint(uint64(r.Metric), 10))
		}
		return b.String()
	}

	if r.Dest == 0 && r.Mask == 0 {
		b.WriteString("default")
	} else {
		b.WriteString(procnetroute.HexToIPStr(r.Dest))
		prefixLen := procnetroute.Popcount(r.Mask)
		if prefixLen != 32 {
			b.WriteByte('/')
			b.WriteString(strconv.Itoa(prefixLen))
		}
	}

	if r.Flags&procnetroute.FlagGateway != 0 {
		b.WriteString(" via ")
		b.WriteString(procnetroute.HexToIPStr(r.GW))
	}

	b.WriteString(" dev ")
	b.WriteString(r.Iface)

	if r.Metric != 0 {
		b.WriteString(" metric ")
		b.WriteString(strconv.FormatUint(uint64(r.Metric), 10))
	}

	return b.String()
}

// parseIPv4 converts a dotted-decimal IPv4 string to the /proc/net/route
// little-endian uint32 encoding: first octet → lowest byte of the uint32.
func parseIPv4(s string) (uint32, bool) {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return 0, false
	}
	var val uint32
	for i, part := range parts {
		// Reject leading zeros (e.g. "010") — real ip rejects them as invalid.
		if len(part) > 1 && part[0] == '0' {
			return 0, false
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil || n > 255 {
			return 0, false
		}
		val |= uint32(n) << (uint(i) * 8)
	}
	return val, true
}
