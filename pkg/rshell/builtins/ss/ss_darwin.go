// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package ss

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

// run is the macOS implementation. It reads socket state via sysctl.
func run(ctx context.Context, callCtx *builtins.CallContext, opts options) builtins.Result {
	var entries []socketEntry

	if opts.showTCP {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}
		if !opts.ipv6Only {
			if err := collectSysctlTCP("net.inet.tcp.pcblist_n", sockTCP4, &entries); err != nil {
				callCtx.Errf("ss: tcp4: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}
		if !opts.ipv4Only {
			if err := collectSysctlTCP("net.inet6.tcp6.pcblist_n", sockTCP6, &entries); err != nil {
				// IPv6 sysctl is absent on IPv4-only hosts (ENOENT); silently
				// skip. Log unexpected errors (e.g. permission denied) so
				// genuine failures are visible to the caller.
				if !errors.Is(err, syscall.ENOENT) {
					callCtx.Errf("ss: tcp6 (partial): %v\n", err)
				}
			}
		}
	}
	if opts.showUDP {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}
		if !opts.ipv6Only {
			if err := collectSysctlUDP("net.inet.udp.pcblist_n", sockUDP4, &entries); err != nil {
				callCtx.Errf("ss: udp4: %v\n", err)
				return builtins.Result{Code: 1}
			}
		}
		if !opts.ipv4Only {
			if err := collectSysctlUDP("net.inet6.udp6.pcblist_n", sockUDP6, &entries); err != nil {
				// IPv6 sysctl is absent on IPv4-only hosts (ENOENT); silently
				// skip. Log unexpected errors (e.g. permission denied) so
				// genuine failures are visible to the caller.
				if !errors.Is(err, syscall.ENOENT) {
					callCtx.Errf("ss: udp6 (partial): %v\n", err)
				}
			}
		}
	}
	if opts.showUnix {
		if ctx.Err() != nil {
			return builtins.Result{Code: 1}
		}
		if err := collectSysctlUnix("net.local.stream.pcblist_n", &entries); err != nil {
			callCtx.Errf("ss: unix stream: %v\n", err)
			return builtins.Result{Code: 1}
		}
	}

	if opts.summary {
		printSummary(callCtx, entries)
		return builtins.Result{}
	}

	printHeader(callCtx, opts)
	for _, e := range entries {
		if filterEntry(opts, e) {
			printEntry(callCtx, opts, e)
		}
	}
	return builtins.Result{}
}

// XNU sysctl pcblist_n record kind constants.
const (
	kindXinpcb  = 0x10  // xinpcb_n: PCB address/port info
	kindXtcpcb  = 0x20  // xtcpcb_n: TCP state
	kindXrcvbuf = 0x02  // xrcvbuf: receive buffer stats
	kindXsndbuf = 0x04  // xsndbuf: send buffer stats
	kindXunpcb  = 0x200 // xunpcb_n: Unix domain socket PCB
)

// macOS TCP state values from the XNU tcp_fsm.h TCPS_* enum.
// TCPS_CLOSED=0, TCPS_LISTEN=1, TCPS_SYN_SENT=2, TCPS_SYN_RECEIVED=3,
// TCPS_ESTABLISHED=4, TCPS_CLOSE_WAIT=5, TCPS_FIN_WAIT_1=6,
// TCPS_CLOSING=7, TCPS_LAST_ACK=8, TCPS_FIN_WAIT_2=9, TCPS_TIME_WAIT=10.
var darwinTCPStates = map[int32]string{
	0:  "CLOSE",
	1:  "LISTEN",
	2:  "SYN-SENT",
	3:  "SYN-RECV",
	4:  "ESTAB",
	5:  "CLOSE-WAIT",
	6:  "FIN-WAIT-1",
	7:  "CLOSING",
	8:  "LAST-ACK",
	9:  "FIN-WAIT-2",
	10: "TIME-WAIT",
}

// readU16BE reads a big-endian uint16 from data at offset off, returning 0 if
// the slice is too short.
func readU16BE(data []byte, off int) uint16 {
	if off+2 > len(data) {
		return 0
	}
	return uint16(data[off])<<8 | uint16(data[off+1])
}

// readI32LE reads a little-endian int32 from data at offset off, returning 0
// if the slice is too short.
func readI32LE(data []byte, off int) int32 {
	if off+4 > len(data) {
		return 0
	}
	return int32(data[off]) |
		int32(data[off+1])<<8 |
		int32(data[off+2])<<16 |
		int32(data[off+3])<<24
}

// readU32LE reads a little-endian uint32 from data at offset off.
func readU32LE(data []byte, off int) uint32 {
	if off+4 > len(data) {
		return 0
	}
	return uint32(data[off]) |
		uint32(data[off+1])<<8 |
		uint32(data[off+2])<<16 |
		uint32(data[off+3])<<24
}

// formatIPv4Net formats 4 bytes starting at data[off] as an IPv4 address in
// network byte order (big-endian).
func formatIPv4Net(data []byte, off int) string {
	if off+4 > len(data) {
		return "0.0.0.0"
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		data[off], data[off+1], data[off+2], data[off+3])
}

// collectSysctlTCP reads TCP PCBs from the given sysctl key and appends
// entries of the given kind (sockTCP4 or sockTCP6).
//
// The data format is:
//   - 24-byte xinpgen header
//   - Repeated groups of records: xinpcb_n(0x10) + xsocket_n(0x01) +
//     xrcvbuf(0x02) + xsndbuf(0x04) + xstats(0x08) + xtcpcb_n(0x20)
//   - 4-byte zero sentinel between connection groups
func collectSysctlTCP(key string, kind socketType, out *[]socketEntry) error {
	data, err := unix.SysctlRaw(key)
	if err != nil {
		return fmt.Errorf("sysctl %s: %w", key, err)
	}
	if len(data) < 24 {
		return nil
	}

	off := 24 // skip xinpgen header
	for off+8 <= len(data) {
		recLen := int(readU32LE(data, off))
		recKind := readU32LE(data, off+4)

		if recLen == 0 {
			// 4-byte zero sentinel between connection groups.
			off += 4
			continue
		}
		if off+recLen > len(data) {
			break
		}

		switch recKind {
		case kindXinpcb:
			// Extract port and address from xinpcb_n.
			// Offsets confirmed by live kernel probing:
			//   fport @16 (big-endian u16), lport @18 (big-endian u16)
			//   foreign IPv4 @60 (network order), local IPv4 @76 (network order)
			rec := data[off : off+recLen]
			fport := readU16BE(rec, 16)
			lport := readU16BE(rec, 18)
			var faddr, laddr string
			if kind == sockTCP6 {
				faddr = formatIPv6FromBytes(rec, 44)
				laddr = formatIPv6FromBytes(rec, 60)
			} else {
				faddr = formatIPv4Net(rec, 60)
				laddr = formatIPv4Net(rec, 76)
			}

			// Scan forward for xtcpcb_n and buffer records in this group.
			state := "CLOSE"
			var recvQ, sendQ uint64

			inner := off + recLen
			for inner+8 <= len(data) {
				iLen := int(readU32LE(data, inner))
				iKind := readU32LE(data, inner+4)
				if iLen == 0 {
					break // end of this connection group
				}
				if inner+iLen > len(data) {
					break
				}
				switch iKind {
				case kindXtcpcb:
					// t_state at offset 36 (little-endian i32).
					irec := data[inner : inner+iLen]
					tstate := readI32LE(irec, 36)
					if s, ok := darwinTCPStates[tstate]; ok {
						state = s
					}
				case kindXrcvbuf:
					// sb_cc at offset 8 (little-endian u32).
					irec := data[inner : inner+iLen]
					recvQ = uint64(readU32LE(irec, 8))
				case kindXsndbuf:
					irec := data[inner : inner+iLen]
					sendQ = uint64(readU32LE(irec, 8))
				}
				inner += iLen
				if iKind == kindXtcpcb {
					break // xtcpcb_n is always last in the group
				}
			}

			*out = append(*out, socketEntry{
				kind:      kind,
				state:     state,
				recvQ:     recvQ,
				sendQ:     sendQ,
				localAddr: laddr,
				localPort: strconv.Itoa(int(lport)),
				peerAddr:  faddr,
				peerPort:  strconv.Itoa(int(fport)),
			})
		}

		off += recLen
	}
	return nil
}

// collectSysctlUDP reads UDP PCBs from the given sysctl key and appends
// entries. UDP sockets have no TCP state record; their state is always UNCONN
// (unconnected/bound) or ESTAB (connected to a specific remote).
func collectSysctlUDP(key string, kind socketType, out *[]socketEntry) error {
	data, err := unix.SysctlRaw(key)
	if err != nil {
		return fmt.Errorf("sysctl %s: %w", key, err)
	}
	if len(data) < 24 {
		return nil
	}

	off := 24
	for off+8 <= len(data) {
		recLen := int(readU32LE(data, off))
		recKind := readU32LE(data, off+4)

		if recLen == 0 {
			off += 4
			continue
		}
		if off+recLen > len(data) {
			break
		}

		if recKind == kindXinpcb {
			rec := data[off : off+recLen]
			fport := readU16BE(rec, 16)
			lport := readU16BE(rec, 18)
			var faddr, laddr string
			if kind == sockUDP6 {
				faddr = formatIPv6FromBytes(rec, 44)
				laddr = formatIPv6FromBytes(rec, 60)
			} else {
				faddr = formatIPv4Net(rec, 60)
				laddr = formatIPv4Net(rec, 76)
			}

			// UDP state: ESTAB if connected to a specific peer, UNCONN otherwise.
			state := "UNCONN"
			if fport != 0 {
				state = "ESTAB"
			}

			*out = append(*out, socketEntry{
				kind:      kind,
				state:     state,
				localAddr: laddr,
				localPort: strconv.Itoa(int(lport)),
				peerAddr:  faddr,
				peerPort:  strconv.Itoa(int(fport)),
			})
		}

		off += recLen
	}
	return nil
}

// collectSysctlUnix reads Unix domain socket PCBs from the given sysctl key
// and appends entries.
//
// Each xunpcb_n record (kind=0x200, size=588) contains:
//   - Local sun_len @76, sun_family @77, sun_path @78 (null-terminated)
//   - Peer  sun_len @332, sun_family @333, sun_path @334 (null-terminated)
func collectSysctlUnix(key string, out *[]socketEntry) error {
	data, err := unix.SysctlRaw(key)
	if err != nil {
		return fmt.Errorf("sysctl %s: %w", key, err)
	}
	if len(data) < 24 {
		return nil
	}

	off := 24
	for off+8 <= len(data) {
		recLen := int(readU32LE(data, off))
		recKind := readU32LE(data, off+4)

		if recLen == 0 {
			off += 4
			continue
		}
		if off+recLen > len(data) {
			break
		}

		if recKind == kindXunpcb {
			rec := data[off : off+recLen]
			localPath := readSunPath(rec, 78)
			peerPath := readSunPath(rec, 334)

			// Determine state: if peer has an address, the socket is connected (ESTAB).
			// If local has an address, it may be a listening server socket (LISTEN).
			state := "UNCONN"
			localSunLen := uint8(0)
			if 76 < len(rec) {
				localSunLen = rec[76]
			}
			peerSunLen := uint8(0)
			if 332 < len(rec) {
				peerSunLen = rec[332]
			}

			if peerSunLen > 0 {
				state = "ESTAB"
			} else if localSunLen > 0 {
				// A server socket with a local path but no peer is listening.
				state = "LISTEN"
			}

			peerAddr := "*"
			if peerPath != "" {
				peerAddr = peerPath
			}

			*out = append(*out, socketEntry{
				kind:      sockUnix,
				state:     state,
				localAddr: localPath,
				peerAddr:  peerAddr,
			})
		}

		off += recLen
	}
	return nil
}

// readSunPath reads a null-terminated string from data starting at off. Returns
// empty string if the field is empty or out of bounds.
func readSunPath(data []byte, off int) string {
	if off >= len(data) {
		return ""
	}
	end := off
	for end < len(data) && data[end] != 0 {
		end++
	}
	return string(data[off:end])
}

// formatIPv6FromBytes formats 16 bytes starting at data[off] as an IPv6
// address in network byte order.
//
// NOTE: This function implements the same RFC 5952 longest-run "::" compression
// algorithm as formatIPv6 in ss_linux.go. The two differ only in their input
// type: formatIPv6 takes a [16]byte value (from /proc/net parsing), while this
// function takes a []byte slice with an offset (for sysctl record parsing). A
// shared helper is possible but would require a platform-neutral file; fuzz
// coverage on both functions makes the duplication low-risk.
func formatIPv6FromBytes(data []byte, off int) string {
	if off+16 > len(data) {
		return "::"
	}
	var b [16]byte
	copy(b[:], data[off:off+16])

	// Build groups.
	var g [8]uint16
	for i := range g {
		g[i] = uint16(b[i*2])<<8 | uint16(b[i*2+1])
	}

	// Find the longest run of consecutive zero groups.
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
			sb.WriteString("::")
			i += bestLen
			continue
		}
		if i > 0 && !(bestLen > 1 && i == bestStart+bestLen) {
			sb.WriteByte(':')
		}
		sb.WriteString(strconv.FormatUint(uint64(g[i]), 16))
		i++
	}
	return sb.String()
}
