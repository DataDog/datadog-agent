// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package guiimpl

import (
	"encoding/binary"
	"fmt"
	"math/bits"
	"net"
	"unsafe"

	"golang.org/x/sys/cpu"
	"golang.org/x/sys/windows"
)

// tcpTableOwnerPIDAll requests TCP_TABLE_OWNER_PID_ALL from
// GetExtendedTcpTable: one row per connection, each tagged with its owning
// PID.
// https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getextendedtcptable
const tcpTableOwnerPIDAll = 5

var (
	iphlpapiDLL             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTCPTable = iphlpapiDLL.NewProc("GetExtendedTcpTable")
)

type mibTCPRowOwnerPID struct {
	state      uint32
	localAddr  uint32
	localPort  uint32
	remoteAddr uint32
	remotePort uint32
	pid        uint32
}

type mibTCPTableOwnerPID struct {
	numEntries uint32
	table      [1]mibTCPRowOwnerPID
}

type mibTCP6RowOwnerPID struct {
	localAddr   [16]byte
	localScope  uint32
	localPort   uint32
	remoteAddr  [16]byte
	remoteScope uint32
	remotePort  uint32
	state       uint32
	pid         uint32
}

type mibTCP6TableOwnerPID struct {
	numEntries uint32
	table      [1]mibTCP6RowOwnerPID
}

// portFromField decodes a port number stored, like the rest of the fields
// GetExtendedTcpTable returns, in network byte order within the low 16 bits
// of a 32-bit field.
func portFromField(v uint32) uint16 {
	if !cpu.IsBigEndian {
		return uint16(bits.ReverseBytes32(v) >> 16)
	}
	return uint16(v >> 16)
}

// addrFromV4Field decodes an IPv4 address from a MIB_TCPROW_OWNER_PID
// address field. Unlike the port fields above, the address occupies the
// field's full 32 bits: re-serializing the value with the same byte order
// used to read it out of the raw table buffer reconstructs the original,
// already-network-order byte sequence.
func addrFromV4Field(v uint32) net.IP {
	buf := make([]byte, 4)
	if cpu.IsBigEndian {
		binary.BigEndian.PutUint32(buf, v)
	} else {
		binary.LittleEndian.PutUint32(buf, v)
	}
	return net.IP(buf)
}

// getExtendedTCPTable calls GetExtendedTcpTable for the given address
// family, growing the buffer until the kernel-reported size is satisfied.
func getExtendedTCPTable(family uint32) ([]byte, error) {
	var size uint32
	var buf []byte
	for {
		var addr unsafe.Pointer
		if len(buf) > 0 {
			addr = unsafe.Pointer(&buf[0])
		}
		ret, _, _ := procGetExtendedTCPTable.Call(
			uintptr(addr),
			uintptr(unsafe.Pointer(&size)),
			1, // sorted
			uintptr(family),
			tcpTableOwnerPIDAll,
			0,
		)
		if ret == 0 {
			return buf[:size], nil
		}
		if windows.Errno(ret) != windows.ERROR_INSUFFICIENT_BUFFER {
			return nil, windows.Errno(ret)
		}
		const maxTableSize = 10 << 20
		if size < 4 || size > maxTableSize {
			return nil, fmt.Errorf("unreasonable table size %d reported by GetExtendedTcpTable", size)
		}
		buf = make([]byte, size)
	}
}

func findPIDInV4Table(buf []byte, localPort, remotePort int, localAddr, remoteAddr net.IP) (uint32, bool) {
	if len(buf) == 0 {
		return 0, false
	}
	info := (*mibTCPTableOwnerPID)(unsafe.Pointer(&buf[0]))
	rows := unsafe.Slice(&info.table[0], info.numEntries)
	for i := range rows {
		if int(portFromField(rows[i].localPort)) == localPort &&
			int(portFromField(rows[i].remotePort)) == remotePort &&
			addrFromV4Field(rows[i].localAddr).Equal(localAddr) &&
			addrFromV4Field(rows[i].remoteAddr).Equal(remoteAddr) {
			return rows[i].pid, true
		}
	}
	return 0, false
}

func findPIDInV6Table(buf []byte, localPort, remotePort int, localAddr, remoteAddr net.IP) (uint32, bool) {
	if len(buf) == 0 {
		return 0, false
	}
	info := (*mibTCP6TableOwnerPID)(unsafe.Pointer(&buf[0]))
	rows := unsafe.Slice(&info.table[0], info.numEntries)
	for i := range rows {
		if int(portFromField(rows[i].localPort)) == localPort &&
			int(portFromField(rows[i].remotePort)) == remotePort &&
			net.IP(rows[i].localAddr[:]).Equal(localAddr) &&
			net.IP(rows[i].remoteAddr[:]).Equal(remoteAddr) {
			return rows[i].pid, true
		}
	}
	return 0, false
}

// lookupLoopbackPeerIdentity finds the security identifier (SID) of the
// process holding the local end of the loopback TCP connection whose local
// address/port is peerAddr/peerPort and whose remote address/port is
// serverAddr/serverPort. Matching on ports alone isn't enough: two loopback
// connections can share a local/remote port pair across address families
// (IPv4 vs IPv6) or distinct loopback addresses, which would let an
// unrelated connection's SID be returned instead. peerAddr's own family,
// rather than trying IPv4 then falling back to IPv6, decides which table to
// query.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	if v4 := peerAddr.To4(); v4 != nil {
		table, err := getExtendedTCPTable(windows.AF_INET)
		if err != nil {
			return "", fmt.Errorf("failed to get IPv4 TCP table: %w", err)
		}
		if pid, ok := findPIDInV4Table(table, peerPort, serverPort, v4, serverAddr); ok {
			return sidForPID(pid)
		}
		return "", fmt.Errorf("no matching IPv4 TCP connection for local port %d, remote port %d", peerPort, serverPort)
	}

	table, err := getExtendedTCPTable(windows.AF_INET6)
	if err != nil {
		return "", fmt.Errorf("failed to get IPv6 TCP table: %w", err)
	}
	if pid, ok := findPIDInV6Table(table, peerPort, serverPort, peerAddr, serverAddr); ok {
		return sidForPID(pid)
	}
	return "", fmt.Errorf("no matching IPv6 TCP connection for local port %d, remote port %d", peerPort, serverPort)
}

// sidForPID returns the string form of the SID of the user owning pid.
// LookupAccount's friendly-name resolution is deliberately skipped: only
// equality comparison is needed, not a human-readable name.
//
// This is subject to a residual PID-reuse race: GetExtendedTcpTable (called
// by lookupLoopbackPeerIdentity, above) reports only a bare PID for each
// connection, not a stable handle or the process's creation time, and by the
// time OpenProcess resolves that PID here, the original process may have
// exited and the PID been recycled by an unrelated process. Unlike Linux's
// /proc/net/tcp and Darwin's pcblist64 sysctl, which both expose the owning
// UID directly in the same connection-table snapshot, Windows offers no
// public API to read the owning SID atomically with the connection lookup,
// so this window can't be closed outright. It's accepted as residual risk:
// the race requires winning it within the lifetime of a single, just-minted,
// 30-second intent token, and still only grants the recycled process's own
// identity, not arbitrary privilege escalation.
func sidForPID(pid uint32) (peerIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", fmt.Errorf("failed to open process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return "", fmt.Errorf("failed to open process token for pid %d: %w", pid, err)
	}
	defer token.Close()

	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("failed to get token user for pid %d: %w", pid, err)
	}

	return peerIdentity(tokenUser.User.Sid.String()), nil
}
