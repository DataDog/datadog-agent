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

// consoleDevicePath is stat'd to resolve the console user's UID; a var so tests can point it at a fixture instead of the real /dev/console.
var consoleDevicePath = "/dev/console"

// elevatedMintIdentity resolves a root mint-time identity to the console user's UID via /dev/console's owner (chowned by loginwindow on login), since sudo-minted tokens are typically redeemed by that user's browser, not root's.
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

// identityFromConsoleUID treats a stat failure or UID 0 (nobody logged in, or genuinely root) as undeterminable, falling back to unconstrained.
func identityFromConsoleUID(uid uint32, ok bool) peerIdentity {
	if !ok || uid == 0 {
		return ""
	}
	return peerIdentity(strconv.FormatUint(uint64(uid), 10))
}

// Layout of struct xtcpcb64, as returned by the "net.inet.tcp.pcblist64" sysctl; packed under a non-default #pragma pack, so offsets don't follow ordinary LP64 alignment and were obtained via offsetof()/sizeof() against real SDK headers, not derived by hand.
const (
	// xtcpcb64RecordSize is a well-formed PCB record's exact byte size; a different self-declared length means it's the xinpgen header/trailer, not a PCB.
	xtcpcb64RecordSize = 472
	// xtcpcb64FportOffset is xt_inpcb.inp_fport: the peer's port, network byte order (matches netstat(1)'s own ntohs() use).
	xtcpcb64FportOffset = 20
	// xtcpcb64LportOffset is xt_inpcb.inp_lport: the local port, network byte order.
	xtcpcb64LportOffset = 22
	// xtcpcb64SoUIDOffset is xt_inpcb.xi_socket.so_uid: the owning UID, a plain native-endian integer, not a network-order protocol field.
	xtcpcb64SoUIDOffset = 252
	// xtcpcb64VflagOffset is xt_inpcb.inp_vflag: a bitmask (INP_IPV4=0x1, INP_IPV6=0x2) saying which local-address field below is valid.
	xtcpcb64VflagOffset = 96
	// xtcpcb64Laddr4Offset is xt_inpcb.inp_laddr: the 4-byte IPv4 local address, valid only when INP_IPV4 is set.
	xtcpcb64Laddr4Offset = 128
	// xtcpcb64Laddr6Offset is xt_inpcb.in6p_laddr: the 16-byte IPv6 local address, valid only when INP_IPV6 is set.
	xtcpcb64Laddr6Offset = 116
	// xtcpcb64Faddr4Offset is xt_inpcb.inp_faddr: the 4-byte IPv4 foreign (peer) address, valid only when INP_IPV4 is set.
	xtcpcb64Faddr4Offset = 112
	// xtcpcb64Faddr6Offset is xt_inpcb.in6p_faddr: the 16-byte IPv6 foreign address, valid only when INP_IPV6 is set.
	xtcpcb64Faddr6Offset = 100
	// recordLenFieldSize is the leading length prefix (xt_len/xig_len) that every record in the sysctl's output starts with.
	recordLenFieldSize = 4

	inpIPv4 = 0x1
	inpIPv6 = 0x2
)

// lookupLoopbackPeerIdentity finds the UID owning the loopback TCP connection via the "net.inet.tcp.pcblist64" sysctl (readable regardless of caller UID), not via lsof/PID enumeration (proc_pidinfo is restricted to the same UID/root and can't see another user's FDs).
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist64")
	if err != nil {
		return "", fmt.Errorf("net.inet.tcp.pcblist64: %w", err)
	}
	return findUIDInPCBList(buf, serverAddr, serverPort, peerPort, peerAddr)
}

// findUIDInPCBList walks the "net.inet.tcp.pcblist64" sysctl's raw output for the matching connection; split out from lookupLoopbackPeerIdentity so it's unit-testable without a real syscall.
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
			// Matching on ports alone isn't enough: two loopback connections can share a port pair across address families/addresses, misattributing an unrelated connection's UID.
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

// localAddrFromPCBRecord reads xt_inpcb's local address out of a PCB record, picking the IPv4 or IPv6 field by inp_vflag; nil if neither flag is set.
func localAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Laddr4Offset, xtcpcb64Laddr6Offset)
}

// foreignAddrFromPCBRecord reads xt_inpcb's foreign (peer) address, the same way localAddrFromPCBRecord reads the local one.
func foreignAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Faddr4Offset, xtcpcb64Faddr6Offset)
}

// addrFromPCBRecord reads one of xt_inpcb's address fields by inp_vflag (nil if neither flag is set); the IPv4-mapped address xnu stores in the IPv6 field is byte-for-byte the same as the plain IPv4 one, so either matched case is equivalent for net.IP.Equal.
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
