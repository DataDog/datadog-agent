// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package ss implements the ss builtin command.
//
// ss — socket statistics
//
// Usage: ss [OPTION]...
//
// Display information about network sockets. Reads kernel socket state
// directly without executing any external binary. On Linux the data comes
// from /proc/net/tcp, /proc/net/tcp6, /proc/net/udp, /proc/net/udp6, and
// /proc/net/unix via os.Open directly (AllowedPaths sandbox is not used;
// the paths are derived from ProcPath, a hardcoded kernel pseudo-filesystem
// root that is never derived from user input). On macOS kernel data is read
// via syscall.SysctlRaw (no
// unsafe at the call site). On Windows a narrow unsafe exception is used
// to call GetExtendedTcpTable via iphlpapi.dll.
//
// Accepted flags:
//
//	-t, --tcp
//	    Display only TCP sockets.
//
//	-u, --udp
//	    Display only UDP sockets.
//
//	-x, --unix
//	    Display only Unix domain sockets.
//
//	-l, --listening
//	    Display only listening (bound) sockets. By default only
//	    non-listening sockets are shown; -l reverses that.
//
//	-a, --all
//	    Display all sockets regardless of state (listening and
//	    non-listening). Overrides the default non-listening-only filter.
//
//	-n, --numeric
//	    Do not resolve service names or hostnames; display numeric
//	    addresses and port numbers.
//
//	-4, --ipv4
//	    Display only IPv4 sockets (TCP and UDP). Has no effect on Unix
//	    domain sockets.
//
//	-6, --ipv6
//	    Display only IPv6 sockets (TCP and UDP). Has no effect on Unix
//	    domain sockets.
//
//	-s, --summary
//	    Print a one-page summary of socket statistics and exit. No
//	    per-socket rows are printed.
//
//	-H, --no-header
//	    Suppress the column header line.
//
//	-o, --options
//	    Show per-socket timer information as a timer:(...) suffix.
//
//	-e, --extended
//	    Show extended socket information: UID and inode number per socket.
//
//	-h, --help
//	    Print usage to stdout and exit 0.
//
// Rejected flags: -F/--filter (GTFOBins file read), -p/--processes (PID
// disclosure), -K/--kill (writes to kernel), -E/--events (infinite stream),
// -N/--net (namespace switching), -b/--bpf, -r/--resolve (DNS),
// -m/--memory, -Z/-z (SELinux), -d/-w/-S/-0 (niche protocols).
//
// Default filter behaviour:
//
//	No -t/-u/-x specified    → show all socket types (TCP + UDP + Unix)
//	No -a, no -l             → show non-listening sockets only
//	-l                       → show listening sockets only
//	-a                       → show all sockets (listening + non-listening)
//	-4 / -6                  → restrict TCP/UDP to the specified IP version
//
// Exit codes:
//
//	0  Success (even if no sockets match the filter).
//	1  An error occurred (unreadable proc file, invalid argument, etc.).
//
// Memory safety:
//
//	Linux: /proc/net/ files are finite. Input is read line-by-line via
//	bufio.Scanner with a MaxLineBytes cap. ctx.Err() is checked at the
//	top of every scan loop.
//
//	macOS: sysctl returns a bounded []byte. Every offset dereference is
//	bounds-checked against len(data) before reading.
//
//	Windows: the DLL grow-loop is capped at winnet.MaxBufSize (5 MiB).
//	unsafe.Pointer is used only to pass &buf[0] to the DLL call; the
//	returned data is parsed entirely with encoding/binary.LittleEndian.
package ss

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// Cmd is the ss builtin command descriptor.
var Cmd = builtins.Command{
	Name:        "ss",
	Description: "display socket statistics",
	MakeFlags:   registerFlags,
}

// MaxLineBytes is the per-line buffer cap for the Linux /proc/net/ scanner.
const MaxLineBytes = 1 << 20 // 1 MiB

// socketType identifies the protocol family of a socket entry.
type socketType int

const (
	sockTCP4 socketType = iota
	sockTCP6
	sockUDP4
	sockUDP6
	sockUnix
)

// socketEntry holds the parsed fields common to all socket types.
type socketEntry struct {
	kind      socketType
	state     string
	recvQ     uint64
	sendQ     uint64
	localAddr string
	localPort string
	peerAddr  string
	peerPort  string
	// Extended fields — uid and inode are only populated by the Linux
	// collector (read from /proc/net columns); hasExtended is true only
	// when those fields carry real values so that non-Linux platforms do
	// not emit misleading uid:0 inode:0 output when -e is requested.
	uid         uint32
	inode       uint64
	hasExtended bool
	// Timer info (populated when -o is set).
	timer string
}

// options holds the resolved flag values after pflag parsing.
type options struct {
	showTCP    bool
	showUDP    bool
	showUnix   bool
	showAll    bool // -a: listening + non-listening
	listenOnly bool // -l: listening only
	// numericAddrs is accepted for compatibility (-n/--numeric) but has no
	// runtime effect: this implementation never performs DNS or service-name
	// lookups, so output is always in numeric form.
	numericAddrs bool // -n
	ipv4Only     bool // -4
	ipv6Only     bool // -6
	summary      bool // -s
	noHeader     bool // -H
	showOptions  bool // -o
	extended     bool // -e
}

// registerFlags registers all ss flags on the framework-provided FlagSet and
// returns the bound handler. The framework parses args and passes positional
// arguments (none expected for ss) to the handler.
func registerFlags(fs *builtins.FlagSet) builtins.HandlerFunc {
	help := fs.BoolP("help", "h", false, "print usage and exit")
	tcp := fs.BoolP("tcp", "t", false, "display only TCP sockets")
	udp := fs.BoolP("udp", "u", false, "display only UDP sockets")
	unix := fs.BoolP("unix", "x", false, "display only Unix domain sockets")
	listening := fs.BoolP("listening", "l", false, "display only listening sockets")
	all := fs.BoolP("all", "a", false, "display all sockets (listening and non-listening)")
	numeric := fs.BoolP("numeric", "n", false, "do not resolve service names")
	ipv4 := fs.BoolP("ipv4", "4", false, "display only IPv4 sockets")
	ipv6 := fs.BoolP("ipv6", "6", false, "display only IPv6 sockets")
	summary := fs.BoolP("summary", "s", false, "print summary statistics only")
	noHeader := fs.BoolP("no-header", "H", false, "suppress column header")
	showOpts := fs.BoolP("options", "o", false, "show timer information")
	extended := fs.BoolP("extended", "e", false, "show extended socket info (uid, inode)")

	return func(ctx context.Context, callCtx *builtins.CallContext, _ []string) builtins.Result {
		if *help {
			callCtx.Out("Usage: ss [OPTION]...\n")
			callCtx.Out("Display information about network sockets.\n\n")
			fs.SetOutput(callCtx.Stdout)
			fs.PrintDefaults()
			return builtins.Result{}
		}

		showAny := *tcp || *udp || *unix
		opts := options{
			showTCP:      !showAny || *tcp,
			showUDP:      !showAny || *udp,
			showUnix:     !showAny || *unix,
			showAll:      *all,
			listenOnly:   *listening,
			numericAddrs: *numeric,
			ipv4Only:     *ipv4,
			ipv6Only:     *ipv6,
			summary:      *summary,
			noHeader:     *noHeader,
			showOptions:  *showOpts,
			extended:     *extended,
		}

		return run(ctx, callCtx, opts)
	}
}

// netidStr returns the Netid column string for a socket entry.
func netidStr(e socketEntry) string {
	switch e.kind {
	case sockTCP4, sockTCP6:
		return "tcp"
	case sockUDP4, sockUDP6:
		return "udp"
	case sockUnix:
		return "u_str"
	default:
		return "???"
	}
}

// isListening reports whether the entry represents a listening/bound socket.
// TCP/Unix listening sockets have state "LISTEN"; UDP sockets in UNCONN state
// are considered "listening" (bound but not connected).
// Unix sockets with unknown states are mapped to "UNKNOWN" (not "UNCONN") so
// that they are not mistakenly classified as listening/bound sockets.
func isListening(e socketEntry) bool {
	return e.state == "LISTEN" || e.state == "UNCONN"
}

// filterEntry returns true if the entry should be included in the output
// given the current filter options.
func filterEntry(opts options, e socketEntry) bool {
	// Protocol filter.
	switch e.kind {
	case sockTCP4, sockTCP6:
		if !opts.showTCP {
			return false
		}
	case sockUDP4, sockUDP6:
		if !opts.showUDP {
			return false
		}
	case sockUnix:
		if !opts.showUnix {
			return false
		}
	}

	// IP version filter (TCP/UDP only). When both -4 and -6 are given they
	// act as an inclusive OR: both families are shown (matching ss behaviour).
	if opts.ipv4Only && !opts.ipv6Only && (e.kind == sockTCP6 || e.kind == sockUDP6) {
		return false
	}
	if opts.ipv6Only && !opts.ipv4Only && (e.kind == sockTCP4 || e.kind == sockUDP4) {
		return false
	}

	// Listening state filter.
	listening := isListening(e)
	if opts.showAll {
		return true
	}
	if opts.listenOnly {
		return listening
	}
	// Default: non-listening only.
	return !listening
}

// formatAddrPort returns a formatted "addr:port" string. When port is "0" or
// empty the port is shown as "*".
func formatAddrPort(addr, port string) string {
	if port == "" || port == "0" {
		return addr + ":*"
	}
	return addr + ":" + port
}

// printHeader writes the column header to stdout unless -H was given.
func printHeader(callCtx *builtins.CallContext, opts options) {
	if opts.noHeader {
		return
	}
	hdr := fmt.Sprintf("%-5s  %-10s  %6s  %6s  %-28s %-28s",
		"Netid", "State", "Recv-Q", "Send-Q",
		"Local Address:Port", "Peer Address:Port")
	if opts.extended {
		hdr += "  Extended"
	}
	if opts.showOptions {
		hdr += "  Timer"
	}
	callCtx.Out(hdr + "\n")
}

// printEntry writes one socket row to stdout.
func printEntry(callCtx *builtins.CallContext, opts options, e socketEntry) {
	local := formatAddrPort(e.localAddr, e.localPort)
	peer := formatAddrPort(e.peerAddr, e.peerPort)

	line := fmt.Sprintf("%-5s  %-10s  %6d  %6d  %-28s %-28s",
		netidStr(e), e.state,
		e.recvQ, e.sendQ,
		local, peer)

	if opts.extended && e.hasExtended {
		line += fmt.Sprintf("  uid:%d inode:%d", e.uid, e.inode)
	}
	if opts.showOptions {
		timer := e.timer
		if timer == "" {
			timer = "off"
		}
		line += fmt.Sprintf("  timer:(%s)", timer)
	}

	callCtx.Out(line + "\n")
}

// printSummary writes a summary of socket counts derived from the provided
// entries.
func printSummary(callCtx *builtins.CallContext, entries []socketEntry) {
	var tcp4, tcp6, udp4, udp6, unix int
	for _, e := range entries {
		switch e.kind {
		case sockTCP4:
			tcp4++
		case sockTCP6:
			tcp6++
		case sockUDP4:
			udp4++
		case sockUDP6:
			udp6++
		case sockUnix:
			unix++
		}
	}
	total := tcp4 + tcp6 + udp4 + udp6 + unix
	callCtx.Outf("Total: %d\n", total)
	callCtx.Outf("TCP:   %d (ipv4: %d, ipv6: %d)\n", tcp4+tcp6, tcp4, tcp6)
	callCtx.Outf("UDP:   %d (ipv4: %d, ipv6: %d)\n", udp4+udp6, udp4, udp6)
	callCtx.Outf("Unix:  %d\n", unix)
}
