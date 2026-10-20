// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package procnetsocket

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// tcpStateMap translates the hex state field from /proc/net/tcp* to a
// human-readable state name. States match the Linux tcp_state enum.
var tcpStateMap = map[string]string{
	"01": "ESTAB",
	"02": "SYN-SENT",
	"03": "SYN-RECV",
	"04": "FIN-WAIT-1",
	"05": "FIN-WAIT-2",
	"06": "TIME-WAIT",
	"07": "CLOSE",
	"08": "CLOSE-WAIT",
	"09": "LAST-ACK",
	"0A": "LISTEN",
	"0B": "CLOSING",
	"0C": "NEW-SYN-RECV",
}

// udpStateMap translates the hex state field from /proc/net/udp* to a
// human-readable state name.
var udpStateMap = map[string]string{
	"01": "ESTAB",
	"07": "UNCONN",
}

// unixStateMap translates the hex state field from /proc/net/unix.
// The kernel prints sk_state as %02X; values are TCP state enum entries.
var unixStateMap = map[string]string{
	"01": "ESTAB",  // TCP_ESTABLISHED = 1
	"0A": "LISTEN", // TCP_LISTEN = 10
}

// parseIPv4Proc decodes an 8-hex-digit little-endian IPv4 address from
// /proc/net/tcp or /proc/net/udp into dotted-decimal notation.
// Example: "0100007F" → "127.0.0.1".
func parseIPv4Proc(s string) (string, error) {
	if len(s) != 8 {
		return "", fmt.Errorf("invalid IPv4 hex: %q", s)
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return "", fmt.Errorf("invalid IPv4 hex: %q: %w", s, err)
	}
	// v holds the bytes in little-endian order as a uint32 interpreted big-endian.
	// Extract in byte order: LSB = first octet = highest numeric value.
	return fmt.Sprintf("%d.%d.%d.%d",
		v&0xFF, (v>>8)&0xFF, (v>>16)&0xFF, (v>>24)&0xFF), nil
}

// parsePortProc decodes a 4-hex-digit big-endian port from /proc/net/tcp* or
// /proc/net/udp*.
func parsePortProc(s string) (string, error) {
	v, err := strconv.ParseUint(s, 16, 16)
	if err != nil {
		return "", fmt.Errorf("invalid port hex: %q: %w", s, err)
	}
	return strconv.FormatUint(v, 10), nil
}

// parseIPv6Proc decodes a 32-hex-digit IPv6 address from /proc/net/tcp6 or
// /proc/net/udp6. Each 8-char group is a little-endian uint32 representing
// 4 bytes of the IPv6 address in network byte order.
func parseIPv6Proc(s string) (string, error) {
	if len(s) != 32 {
		return "", fmt.Errorf("invalid IPv6 hex length: %d", len(s))
	}
	var b [16]byte
	for i := range 4 {
		word, err := strconv.ParseUint(s[i*8:(i+1)*8], 16, 32)
		if err != nil {
			return "", fmt.Errorf("invalid IPv6 group: %w", err)
		}
		// Little-endian: LSB of word is the first byte of this group in
		// network (big-endian) order. Mask each octet explicitly so that
		// CodeQL can verify the values are bounded to [0, 255] before the
		// byte conversion and any subsequent widening to uint16.
		b[i*4+0] = byte(word & 0xFF)
		b[i*4+1] = byte((word >> 8) & 0xFF)
		b[i*4+2] = byte((word >> 16) & 0xFF)
		b[i*4+3] = byte((word >> 24) & 0xFF)
	}
	return formatIPv6(b), nil
}

// formatIPv6 converts a 16-byte IPv6 address into condensed notation with "::"
// replacing the longest run of consecutive all-zero 16-bit groups.
func formatIPv6(b [16]byte) string {
	// Build 8 uint16 groups.
	var g [8]uint16
	for i := range g {
		g[i] = uint16(b[i*2])<<8 | uint16(b[i*2+1])
	}

	// Find the longest run of consecutive zero groups (must be > 1 to compress).
	bestStart, bestLen := -1, 0
	for i := 0; i < 8; {
		if g[i] == 0 {
			j := i + 1
			for j < 8 && g[j] == 0 {
				j++
			}
			if j-i > bestLen {
				bestStart, bestLen = i, j-i
			}
			i = j
		} else {
			i++
		}
	}

	var sb strings.Builder
	for i := 0; i < 8; {
		if bestLen > 1 && i == bestStart {
			// Write "::" — serves as both the separator from the previous group
			// (if any) and the compressed zero notation.
			sb.WriteString("::")
			i += bestLen
			continue
		}
		// Separator from the previous group, except immediately after "::"
		// where the "::" already ends with ":".
		if i > 0 && !(bestLen > 1 && i == bestStart+bestLen) {
			sb.WriteByte(':')
		}
		sb.WriteString(strconv.FormatUint(uint64(g[i]), 16))
		i++
	}
	return sb.String()
}

func readTCP4(ctx context.Context, procPath string) ([]SocketEntry, error) {
	return parseProcNetIP(ctx, filepath.Join(procPath, "net", "tcp"), KindTCP4, tcpStateMap, parseIPv4Proc)
}

func readTCP6(ctx context.Context, procPath string) ([]SocketEntry, error) {
	return parseProcNetIP(ctx, filepath.Join(procPath, "net", "tcp6"), KindTCP6, tcpStateMap, parseIPv6Proc)
}

func readUDP4(ctx context.Context, procPath string) ([]SocketEntry, error) {
	return parseProcNetIP(ctx, filepath.Join(procPath, "net", "udp"), KindUDP4, udpStateMap, parseIPv4Proc)
}

func readUDP6(ctx context.Context, procPath string) ([]SocketEntry, error) {
	return parseProcNetIP(ctx, filepath.Join(procPath, "net", "udp6"), KindUDP6, udpStateMap, parseIPv6Proc)
}

func readUnix(ctx context.Context, procPath string) ([]SocketEntry, error) {
	return parseProcNetUnix(ctx, filepath.Join(procPath, "net", "unix"))
}

// parseProcNetIP is the shared parser for /proc/net/tcp*, /proc/net/udp*.
// The format of each non-header line is:
//
//	sl  local_address rem_address st tx_queue:rx_queue ... uid timeout inode ...
//
// Fields are 0-indexed after splitting on whitespace.
//
// Sandbox bypass: os.Open is used directly instead of callCtx.OpenFile because
// path is always derived from procPath (a hardcoded kernel pseudo-filesystem
// root, never from user input). See package doc for rationale.
func parseProcNetIP(
	ctx context.Context,
	path string,
	kind SocketKind,
	stateMap map[string]string,
	parseAddr func(string) (string, error),
) ([]SocketEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), MaxLineBytes)

	var out []SocketEntry
	header := true
	totalLines := 0
	for sc.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if header {
			header = false
			continue
		}
		// MaxEntries is the memory guard: fail once too many entries are held.
		// Returning an error ensures callers know the table is incomplete.
		if len(out) >= MaxEntries {
			return nil, ErrMaxEntries
		}
		// MaxTotalLines is the scan-time guard: bounds CPU time for files with
		// many malformed/skipped lines before MaxEntries valid entries are found.
		// Returning an error ensures callers know the table is incomplete.
		totalLines++
		if totalLines > MaxTotalLines {
			return nil, ErrMaxTotalLines
		}
		line := sc.Text()
		fields := strings.Fields(line)
		// Minimum fields: sl, local_address, rem_address, st, tx:rx, ...
		// uid at index 7, inode at index 9 (for tcp/udp).
		if len(fields) < 10 {
			continue
		}

		// local_address and rem_address: "HEXIP:HEXPORT"
		localParts := strings.Split(fields[1], ":")
		remParts := strings.Split(fields[2], ":")
		if len(localParts) != 2 || len(remParts) != 2 {
			continue
		}

		localAddr, err := parseAddr(localParts[0])
		if err != nil {
			continue
		}
		localPort, err := parsePortProc(localParts[1])
		if err != nil {
			continue
		}
		remAddr, err := parseAddr(remParts[0])
		if err != nil {
			continue
		}
		remPort, err := parsePortProc(remParts[1])
		if err != nil {
			continue
		}

		// State (hex, uppercased).
		stHex := strings.ToUpper(fields[3])
		state, ok := stateMap[stHex]
		if !ok {
			state = "UNKNOWN"
		}

		// tx_queue:rx_queue — hex values.
		var sendQ, recvQ uint64
		qParts := strings.Split(fields[4], ":")
		if len(qParts) == 2 {
			sendQ, _ = strconv.ParseUint(qParts[0], 16, 64)
			recvQ, _ = strconv.ParseUint(qParts[1], 16, 64)
		}

		// uid at field[7], inode at field[9].
		uid64, _ := strconv.ParseUint(fields[7], 10, 32)
		inode, _ := strconv.ParseUint(fields[9], 10, 64)

		out = append(out, SocketEntry{
			Kind:        kind,
			State:       state,
			RecvQ:       recvQ,
			SendQ:       sendQ,
			LocalAddr:   localAddr,
			LocalPort:   localPort,
			PeerAddr:    remAddr,
			PeerPort:    remPort,
			UID:         uint32(uid64),
			Inode:       inode,
			HasExtended: true,
		})
	}
	return out, sc.Err()
}

// parseProcNetUnix reads /proc/net/unix and returns Unix domain socket entries.
// The format of each non-header line is:
//
//	Num RefCount Protocol Flags Type St Inode [Path]
//
// Sandbox bypass: os.Open is used directly; see parseProcNetIP for rationale.
func parseProcNetUnix(ctx context.Context, path string) ([]SocketEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), MaxLineBytes)

	var out []SocketEntry
	header := true
	totalLines := 0
	for sc.Scan() {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if header {
			header = false
			continue
		}
		// MaxEntries is the memory guard: fail once too many entries are held.
		// Returning an error ensures callers know the table is incomplete.
		if len(out) >= MaxEntries {
			return nil, ErrMaxEntries
		}
		// MaxTotalLines is the scan-time guard: bounds CPU time for files with
		// many malformed/skipped lines before MaxEntries valid entries are found.
		// Returning an error ensures callers know the table is incomplete.
		totalLines++
		if totalLines > MaxTotalLines {
			return nil, ErrMaxTotalLines
		}
		line := sc.Text()
		fields := strings.Fields(line)
		// Fields: Num, RefCount, Protocol, Flags, Type, St, Inode, [Path]
		if len(fields) < 7 {
			continue
		}

		stateStr := strings.ToUpper(fields[5])
		state, ok := unixStateMap[stateStr]
		if !ok {
			// Unknown state: use a distinct sentinel so isListening does not
			// misclassify these sockets as UNCONN (bound/listening). Linux
			// /proc/net/unix uses TCP state enum values; only ESTABLISHED(01)
			// and LISTEN(0A) are common, but other values can appear. Mapping
			// them to UNKNOWN keeps them in the default non-listening output
			// rather than incorrectly treating them as bound sockets.
			state = "UNKNOWN"
		}

		inode, _ := strconv.ParseUint(fields[6], 10, 64)

		// The path column is always the last token and may contain spaces
		// (e.g. abstract sockets or paths with spaces). Reconstruct it by
		// joining all remaining fields so that space-containing paths are
		// not truncated to their first word.
		socketPath := ""
		if len(fields) >= 8 {
			socketPath = strings.Join(fields[7:], " ")
		}

		out = append(out, SocketEntry{
			Kind:      KindUnix,
			State:     state,
			LocalAddr: socketPath,
			LocalPort: "",
			PeerAddr:  "*",
			PeerPort:  "",
			Inode:     inode,
			// /proc/net/unix has no uid column; HasExtended stays false
			// to avoid emitting a fabricated uid:0 with -e.
		})
	}
	return out, sc.Err()
}
