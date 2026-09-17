// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build darwin

package guiimpl

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// consoleDevicePath is stat'd to resolve the console user's UID; a var so tests can point it at a fixture.
var consoleDevicePath = "/dev/console"

// elevatedMintIdentity maps a root mint to the console user's UID (via /dev/console's owner), since sudo-minted tokens are redeemed by that user's browser, not root's.
func elevatedMintIdentity() peerIdentity {
	uid, ok := consoleUID()
	return identityFromConsoleUID(uid, ok)
}

func consoleUID() (uint32, bool) {
	info, err := os.Stat(consoleDevicePath)
	if err != nil {
		return 0, false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return stat.Uid, true
}

// identityFromConsoleUID treats a stat failure or UID 0 (nobody logged in, or root) as undeterminable, i.e. unconstrained.
func identityFromConsoleUID(uid uint32, ok bool) peerIdentity {
	if !ok || uid == 0 {
		return ""
	}
	return peerIdentity(strconv.FormatUint(uint64(uid), 10))
}

// Layout of struct xtcpcb64 from the "net.inet.tcp.pcblist64" sysctl; #pragma-packed, so offsets came from offsetof()/sizeof() against SDK headers, not LP64 alignment.
const (
	// xtcpcb64RecordSize is a PCB record's exact byte size; any other length means an xinpgen header/trailer, not a PCB.
	xtcpcb64RecordSize = 472
	// xtcpcb64FportOffset is xt_inpcb.inp_fport: the peer's port, network byte order.
	xtcpcb64FportOffset = 20
	// xtcpcb64LportOffset is xt_inpcb.inp_lport: the local port, network byte order.
	xtcpcb64LportOffset = 22
	// xtcpcb64SoUIDOffset is xt_inpcb.xi_socket.so_uid: the owning UID, native-endian, not a protocol field.
	xtcpcb64SoUIDOffset = 252
	// xtcpcb64VflagOffset is xt_inpcb.inp_vflag: bitmask (INP_IPV4=0x1, INP_IPV6=0x2) of which local-address field is valid.
	xtcpcb64VflagOffset = 96
	// xtcpcb64Laddr4Offset is xt_inpcb.inp_laddr: 4-byte IPv4 local address, valid when INP_IPV4 is set.
	xtcpcb64Laddr4Offset = 128
	// xtcpcb64Laddr6Offset is xt_inpcb.in6p_laddr: 16-byte IPv6 local address, valid when INP_IPV6 is set.
	xtcpcb64Laddr6Offset = 116
	// xtcpcb64Faddr4Offset is xt_inpcb.inp_faddr: 4-byte IPv4 foreign (peer) address, valid when INP_IPV4 is set.
	xtcpcb64Faddr4Offset = 112
	// xtcpcb64Faddr6Offset is xt_inpcb.in6p_faddr: 16-byte IPv6 foreign address, valid when INP_IPV6 is set.
	xtcpcb64Faddr6Offset = 100
	// recordLenFieldSize is the leading length prefix (xt_len/xig_len) every sysctl record starts with.
	recordLenFieldSize = 4

	inpIPv4 = 0x1
	inpIPv6 = 0x2
)

// lookupLoopbackPeerIdentity finds the UID owning the loopback TCP connection via the "net.inet.tcp.pcblist64" sysctl (readable by any UID), not lsof/PID enumeration (proc_pidinfo can't see another user's FDs).
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist64")
	if err != nil {
		return "", fmt.Errorf("net.inet.tcp.pcblist64: %w", err)
	}
	return findUIDInPCBList(buf, serverAddr, serverPort, peerPort, peerAddr)
}

// findUIDInPCBList walks the pcblist64 raw output for the matching connection; split out so it's unit-testable without a real syscall.
func findUIDInPCBList(buf []byte, serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	offset := 0
	skippedHeader := false
	for offset+recordLenFieldSize <= len(buf) {
		recLen := int(binary.LittleEndian.Uint32(buf[offset:]))
		if recLen <= 0 || offset+recLen > len(buf) {
			break
		}
		if !skippedHeader {
			// The first record is the xinpgen header, not a PCB entry.
			skippedHeader = true
			offset += recLen
			continue
		}
		if recLen == xtcpcb64RecordSize {
			rec := buf[offset : offset+recLen]
			fport := binary.BigEndian.Uint16(rec[xtcpcb64FportOffset:])
			lport := binary.BigEndian.Uint16(rec[xtcpcb64LportOffset:])
			// Ports alone aren't enough: two loopback connections can share a port pair across families/addresses, misattributing the UID.
			if int(lport) == peerPort && int(fport) == serverPort &&
				localAddrFromPCBRecord(rec).Equal(peerAddr) &&
				foreignAddrFromPCBRecord(rec).Equal(serverAddr) {
				uid := binary.LittleEndian.Uint32(rec[xtcpcb64SoUIDOffset:])
				return peerIdentity(strconv.FormatUint(uint64(uid), 10)), nil
			}
		}
		offset += recLen
	}
	return "", fmt.Errorf("no matching TCP connection for local port %d, remote port %d", peerPort, serverPort)
}

// localAddrFromPCBRecord reads xt_inpcb's local address, picking the IPv4/IPv6 field by inp_vflag; nil if neither flag is set.
func localAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Laddr4Offset, xtcpcb64Laddr6Offset)
}

// foreignAddrFromPCBRecord reads xt_inpcb's foreign (peer) address, the same way localAddrFromPCBRecord reads the local one.
func foreignAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Faddr4Offset, xtcpcb64Faddr6Offset)
}

// addrFromPCBRecord reads an xt_inpcb address field by inp_vflag (nil if neither set); xnu's IPv4-mapped v6 form equals the plain IPv4 one under net.IP.Equal.
func addrFromPCBRecord(rec []byte, v4Offset, v6Offset int) net.IP {
	switch vflag := rec[xtcpcb64VflagOffset]; {
	case vflag&inpIPv4 != 0:
		return net.IP(rec[v4Offset : v4Offset+4])
	case vflag&inpIPv6 != 0:
		return net.IP(rec[v6Offset : v6Offset+16])
	default:
		return nil
	}
}
