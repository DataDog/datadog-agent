// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// This file contains a narrow unsafe exception: unsafe.Pointer is used solely
// to pass &buf[0] and &size to GetExtendedTcpTable / GetExtendedUdpTable via
// iphlpapi.dll. No other unsafe operations are performed. The returned buffer
// is parsed entirely with encoding/binary.LittleEndian helpers and standard
// arithmetic — no unsafe pointer arithmetic is used after the DLL call.
package winnet

import (
	"encoding/binary"
	"fmt"
	"syscall"
	"unsafe"
)

const (
	afINET  = 2
	afINET6 = 23

	// TCP_TABLE_OWNER_PID_ALL returns all TCP connections with owning PID.
	tcpTableOwnerPidAll = 5
	// UDP_TABLE_OWNER_PID returns all UDP endpoints with owning PID.
	udpTableOwnerPid = 1

	errInsufficientBuffer = syscall.Errno(122)

	// MaxBufSize is the maximum buffer size allowed in the grow-loop when
	// calling GetExtendedTcpTable / GetExtendedUdpTable. This cap is
	// intentionally defined here (where the DLL calls live) so that the limit
	// stays co-located with the code that enforces it.
	// 5 MiB is sufficient for any realistic socket table on Windows.
	MaxBufSize = 5 << 20 // 5 MiB
)

var (
	iphlpapi            = syscall.MustLoadDLL("iphlpapi.dll")
	getExtendedTcpTable = iphlpapi.MustFindProc("GetExtendedTcpTable")
	getExtendedUdpTable = iphlpapi.MustFindProc("GetExtendedUdpTable")
)

// tcpStateNames maps Windows MIB_TCP_STATE values to ss state strings.
var tcpStateNames = map[uint32]string{
	1:  "CLOSE",
	2:  "LISTEN",
	3:  "SYN-SENT",
	4:  "SYN-RECV",
	5:  "ESTAB",
	6:  "FIN-WAIT-1",
	7:  "FIN-WAIT-2",
	8:  "CLOSE-WAIT",
	9:  "CLOSING",
	10: "LAST-ACK",
	11: "TIME-WAIT",
	12: "CLOSE",
}

// Collect enumerates all TCP and UDP sockets on Windows via iphlpapi.dll.
// The narrow use of unsafe.Pointer is limited to two DLL call sites only.
// If any sub-collection fails, an error is returned immediately so callers
// always receive complete data or a clear failure — never silent partial results.
func Collect() ([]SocketEntry, error) {
	var out []SocketEntry

	// TCP IPv4
	e, err := collectTCP(afINET)
	if err != nil {
		return nil, fmt.Errorf("tcp4: %w", err)
	}
	out = append(out, e...)

	// TCP IPv6
	e, err = collectTCP(afINET6)
	if err != nil {
		return nil, fmt.Errorf("tcp6: %w", err)
	}
	out = append(out, e...)

	// UDP IPv4
	e, err = collectUDP(afINET)
	if err != nil {
		return nil, fmt.Errorf("udp4: %w", err)
	}
	out = append(out, e...)

	// UDP IPv6
	e, err = collectUDP(afINET6)
	if err != nil {
		return nil, fmt.Errorf("udp6: %w", err)
	}
	out = append(out, e...)

	return out, nil
}

// callExtendedTable calls GetExtendedTcpTable or GetExtendedUdpTable with a
// grow-loop, capped at MaxBufSize. Returns the raw buffer on success.
func callExtendedTable(proc *syscall.Proc, af, tableClass uintptr) ([]byte, error) {
	size := uint32(4096)
	for {
		if int(size) > MaxBufSize {
			return nil, fmt.Errorf("buffer size limit exceeded")
		}
		buf := make([]byte, size)
		prevSize := size
		// Narrow unsafe exception: pass &buf[0] as PVOID and &size as PDWORD.
		r1, _, _ := proc.Call(
			uintptr(unsafe.Pointer(&buf[0])), //nolint:govet
			uintptr(unsafe.Pointer(&size)),   //nolint:govet
			0,                                // bOrder: unsorted
			af,                               // ulAf
			tableClass,
			0, // Reserved
		)
		switch syscall.Errno(r1) {
		case 0:
			if int(size) > len(buf) {
				return nil, fmt.Errorf("DLL reported size %d larger than buffer %d", size, len(buf))
			}
			return buf[:size], nil
		case errInsufficientBuffer:
			// size was updated by the call; retry with larger buffer.
			// Guard against a misbehaving DLL that doesn't increase size.
			if size <= prevSize {
				return nil, fmt.Errorf("DLL did not increase buffer size on retry")
			}
			continue
		default:
			return nil, fmt.Errorf("DLL call error: %w", syscall.Errno(r1))
		}
	}
}

// formatIPv4Win formats a network-order (big-endian) 32-bit IPv4 address.
func formatIPv4Win(b []byte, off int) string {
	if off+4 > len(b) {
		return "0.0.0.0"
	}
	return fmt.Sprintf("%d.%d.%d.%d", b[off], b[off+1], b[off+2], b[off+3])
}

// winPortToHost converts a Windows port value (network byte order stored as
// little-endian DWORD) to a host-order uint16.
func winPortToHost(raw uint32) uint16 {
	// Windows stores port as big-endian in the lower 16 bits of a DWORD.
	return uint16(raw>>8) | uint16(raw&0xFF)<<8
}

// collectTCP retrieves TCP socket entries for the given address family.
//
// MIB_TCPROW_OWNER_PID (IPv4, 24 bytes):
//
//	[0..3]   dwState        (LE uint32)
//	[4..7]   dwLocalAddr    (BE uint32)
//	[8..11]  dwLocalPort    (network byte order uint16 in LE DWORD)
//	[12..15] dwRemoteAddr   (BE uint32)
//	[16..19] dwRemotePort   (network byte order uint16 in LE DWORD)
//	[20..23] dwOwningPid    (not used)
//
// MIB_TCP6ROW_OWNER_PID (IPv6, 56 bytes):
//
//	[0..15]  ucLocalAddr[16]    (network byte order)
//	[16..19] dwLocalScopeId     (not used)
//	[20..23] dwLocalPort        (network byte order uint16 in LE DWORD)
//	[24..39] ucRemoteAddr[16]   (network byte order)
//	[40..43] dwRemoteScopeId    (not used)
//	[44..47] dwRemotePort       (network byte order uint16 in LE DWORD)
//	[48..51] dwState            (LE uint32)
//	[52..55] dwOwningPid        (not used)
func collectTCP(af uintptr) ([]SocketEntry, error) {
	data, err := callExtendedTable(getExtendedTcpTable, af, tcpTableOwnerPidAll)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, nil
	}

	numEntries := binary.LittleEndian.Uint32(data[0:4])
	entrySize := uint32(24)
	if af == afINET6 {
		entrySize = 56 // MIB_TCP6ROW_OWNER_PID
	}
	if maxPossible := uint32(MaxBufSize) / entrySize; numEntries > maxPossible {
		numEntries = maxPossible
	}

	var out []SocketEntry
	for i := uint32(0); i < numEntries; i++ {
		off := 4 + i*entrySize
		if int(off)+int(entrySize) > len(data) {
			break
		}

		proto := "tcp4"
		localIP, remoteIP := "", ""
		var localPort, remotePort uint16
		var stateName string

		if af == afINET {
			state := binary.LittleEndian.Uint32(data[off:]) // dwState at offset 0
			var ok bool
			stateName, ok = tcpStateNames[state]
			if !ok {
				stateName = "CLOSE"
			}
			localIP = formatIPv4Win(data, int(off+4))
			localPort = winPortToHost(binary.LittleEndian.Uint32(data[off+8:]))
			remoteIP = formatIPv4Win(data, int(off+12))
			remotePort = winPortToHost(binary.LittleEndian.Uint32(data[off+16:]))
		} else {
			proto = "tcp6"
			state := binary.LittleEndian.Uint32(data[off+48:]) // dwState at offset 48
			var ok bool
			stateName, ok = tcpStateNames[state]
			if !ok {
				stateName = "CLOSE"
			}
			localIP = formatIPv6Win(data, int(off)) // ucLocalAddr at offset 0
			localPort = winPortToHost(binary.LittleEndian.Uint32(data[off+20:]))
			remoteIP = formatIPv6Win(data, int(off+24))                           // ucRemoteAddr at offset 24
			remotePort = winPortToHost(binary.LittleEndian.Uint32(data[off+44:])) // dwRemotePort at offset 44
		}

		out = append(out, SocketEntry{
			Proto:      proto,
			State:      stateName,
			LocalIP:    localIP,
			LocalPort:  localPort,
			RemoteIP:   remoteIP,
			RemotePort: remotePort,
		})
	}
	return out, nil
}

// collectUDP retrieves UDP socket entries for the given address family.
//
// MIB_UDPROW_OWNER_PID (IPv4, 12 bytes):
//
//	[0..3]  dwLocalAddr  (BE uint32)
//	[4..7]  dwLocalPort  (network byte order uint16 in LE DWORD)
//	[8..11] dwOwningPid  (not used)
//
// MIB_UDP6ROW_OWNER_PID (IPv6, 28 bytes):
//
//	[0..15]  ucLocalAddr[16]  (network byte order)
//	[16..19] dwLocalScopeId  (not used)
//	[20..23] dwLocalPort     (network byte order uint16 in LE DWORD)
//	[24..27] dwOwningPid     (not used)
func collectUDP(af uintptr) ([]SocketEntry, error) {
	data, err := callExtendedTable(getExtendedUdpTable, af, udpTableOwnerPid)
	if err != nil {
		return nil, err
	}
	if len(data) < 4 {
		return nil, nil
	}

	numEntries := binary.LittleEndian.Uint32(data[0:4])
	entrySize := uint32(12)
	if af == afINET6 {
		entrySize = 28 // MIB_UDP6ROW_OWNER_PID
	}
	if maxPossible := uint32(MaxBufSize) / entrySize; numEntries > maxPossible {
		numEntries = maxPossible
	}

	var out []SocketEntry
	for i := uint32(0); i < numEntries; i++ {
		off := 4 + i*entrySize
		if int(off)+int(entrySize) > len(data) {
			break
		}

		proto := "udp4"
		localIP := ""
		var localPort uint16

		if af == afINET {
			localIP = formatIPv4Win(data, int(off))
			localPort = winPortToHost(binary.LittleEndian.Uint32(data[off+4:]))
		} else {
			proto = "udp6"
			localIP = formatIPv6Win(data, int(off))                              // ucLocalAddr at offset 0
			localPort = winPortToHost(binary.LittleEndian.Uint32(data[off+20:])) // dwLocalPort at offset 20
		}

		// UDP sockets are connectionless and have no remote address.  Use the
		// correct unspecified address sentinel for the address family: "::" for
		// IPv6, "0.0.0.0" for IPv4.
		remoteIP := "0.0.0.0"
		if af == afINET6 {
			remoteIP = "::"
		}
		out = append(out, SocketEntry{
			Proto:     proto,
			State:     "UNCONN",
			LocalIP:   localIP,
			LocalPort: localPort,
			RemoteIP:  remoteIP,
		})
	}
	return out, nil
}

// formatIPv6Win formats a 16-byte IPv6 address from data at offset off.
func formatIPv6Win(data []byte, off int) string {
	if off+16 > len(data) {
		return "::"
	}
	var g [8]uint16
	for i := range g {
		g[i] = binary.BigEndian.Uint16(data[off+i*2:])
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

	var result [48]byte // enough for a full IPv6 address
	n := 0
	appendHex := func(v uint16) {
		const hex = "0123456789abcdef"
		switch {
		case v >= 0x1000:
			result[n] = hex[v>>12]
			n++
			fallthrough
		case v >= 0x100:
			result[n] = hex[(v>>8)&0xf]
			n++
			fallthrough
		case v >= 0x10:
			result[n] = hex[(v>>4)&0xf]
			n++
		}
		result[n] = hex[v&0xf]
		n++
	}

	for i := 0; i < 8; {
		if bestLen > 1 && i == bestStart {
			result[n] = ':'
			n++
			result[n] = ':'
			n++
			i += bestLen
			continue
		}
		if i > 0 && !(bestLen > 1 && i == bestStart+bestLen) {
			result[n] = ':'
			n++
		}
		appendHex(g[i])
		i++
	}
	return string(result[:n])
}
