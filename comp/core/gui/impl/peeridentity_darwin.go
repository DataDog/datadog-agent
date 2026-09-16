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
	"strconv"

	"golang.org/x/sys/unix"
)

// Layout of struct xtcpcb64, as returned by the "net.inet.tcp.pcblist64"
// sysctl (see <netinet/tcp_var.h>, <netinet/in_pcb.h>, <sys/socketvar.h>).
// These structs are declared under a non-default #pragma pack in the system
// headers, so the offsets below do NOT follow ordinary 8-byte-aligned LP64
// layout; they were obtained by compiling a small probe against the actual
// SDK headers and reading back offsetof()/sizeof(), not derived by hand.
const (
	// xtcpcb64RecordSize is the exact byte size of a well-formed PCB record;
	// any record whose self-declared length differs is the xinpgen
	// header/trailer, not a PCB, and must be skipped.
	xtcpcb64RecordSize = 472
	// xtcpcb64FportOffset is xt_inpcb.inp_fport: the peer's port, in network
	// byte order, matching netstat(1)'s own use of ntohs() on this field.
	xtcpcb64FportOffset = 20
	// xtcpcb64LportOffset is xt_inpcb.inp_lport: the local port, in network
	// byte order.
	xtcpcb64LportOffset = 22
	// xtcpcb64SoUIDOffset is xt_inpcb.xi_socket.so_uid: the owning UID, a
	// plain native-endian (little-endian on both Apple architectures)
	// integer, not a network-order protocol field.
	xtcpcb64SoUIDOffset = 252
	// xtcpcb64VflagOffset is xt_inpcb.inp_vflag: a bitmask (INP_IPV4=0x1,
	// INP_IPV6=0x2) saying which of the two address fields below actually
	// holds the local address.
	xtcpcb64VflagOffset = 96
	// xtcpcb64Laddr4Offset is xt_inpcb.inp_laddr (a #define for
	// inp_dependladdr.inp46_local.ia46_addr4): the 4-byte IPv4 local
	// address, valid only when INP_IPV4 is set.
	xtcpcb64Laddr4Offset = 128
	// xtcpcb64Laddr6Offset is xt_inpcb.in6p_laddr
	// (inp_dependladdr.inp6_local): the 16-byte IPv6 local address, valid
	// only when INP_IPV6 is set.
	xtcpcb64Laddr6Offset = 116
	// xtcpcb64Faddr4Offset is xt_inpcb.inp_faddr (a #define for
	// inp_dependfaddr.inp46_foreign.ia46_addr4): the 4-byte IPv4 foreign
	// (peer, from this record's own point of view) address, valid only when
	// INP_IPV4 is set.
	xtcpcb64Faddr4Offset = 112
	// xtcpcb64Faddr6Offset is xt_inpcb.in6p_faddr
	// (inp_dependfaddr.inp6_foreign): the 16-byte IPv6 foreign address, valid
	// only when INP_IPV6 is set.
	xtcpcb64Faddr6Offset = 100
	// recordLenFieldSize is the size of the leading length prefix (xt_len /
	// xig_len) that every record in the sysctl's output starts with.
	recordLenFieldSize = 4

	inpIPv4 = 0x1
	inpIPv6 = 0x2
)

// lookupLoopbackPeerIdentity finds the UID of the process holding the local
// end of the loopback TCP connection whose local address/port is
// peerAddr/peerPort and whose remote address/port is serverAddr/serverPort,
// using the "net.inet.tcp.pcblist64" sysctl. That table exposes the owning
// UID (so_uid) directly and is readable regardless of the caller's own UID.
// This is deliberately not done via lsof/PID enumeration:
// proc_pidinfo(PROC_PIDLISTFDS), which lsof and gopsutil's darwin backend
// rely on, is restricted by the kernel to the same UID (or root) and cannot
// see another user's open file descriptors, which would defeat the purpose
// of this check.
func lookupLoopbackPeerIdentity(serverAddr net.IP, serverPort, peerPort int, peerAddr net.IP) (peerIdentity, error) {
	buf, err := unix.SysctlRaw("net.inet.tcp.pcblist64")
	if err != nil {
		return "", fmt.Errorf("net.inet.tcp.pcblist64: %w", err)
	}
	return findUIDInPCBList(buf, serverAddr, serverPort, peerPort, peerAddr)
}

// findUIDInPCBList walks the "net.inet.tcp.pcblist64" sysctl's raw output
// (a leading xinpgen header record followed by one variable-format record
// per TCP connection) looking for the connection whose local address/port is
// peerAddr/peerPort and remote address/port is serverAddr/serverPort. It's
// split out from lookupLoopbackPeerIdentity so the buffer-walking logic can
// be unit tested without a real syscall.
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
			// Matching on ports alone isn't enough: two loopback connections
			// can share a local/remote port pair across address families or
			// distinct loopback addresses, which would let an unrelated
			// connection's UID be returned instead.
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

// localAddrFromPCBRecord reads xt_inpcb's local address out of a PCB record,
// picking the IPv4 or IPv6 field according to inp_vflag. It returns nil if
// neither address-family flag is set.
func localAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Laddr4Offset, xtcpcb64Laddr6Offset)
}

// foreignAddrFromPCBRecord reads xt_inpcb's foreign (peer) address out of a
// PCB record, the same way localAddrFromPCBRecord reads the local one.
func foreignAddrFromPCBRecord(rec []byte) net.IP {
	return addrFromPCBRecord(rec, xtcpcb64Faddr4Offset, xtcpcb64Faddr6Offset)
}

// addrFromPCBRecord reads one of xt_inpcb's address fields out of a PCB
// record, picking the IPv4 or IPv6 offset according to inp_vflag. It returns
// nil if neither address-family flag is set. A dual-stack listener accepting
// an IPv4 peer sets both flags; inp_laddr/inp_faddr and
// in6p_laddr/in6p_faddr are a real BSD union (in_dependaddr) in the kernel's
// own layout, so the IPv4-mapped address xnu stores in the IPv6 field
// (e.g. ::ffff:127.0.0.1) occupies the very same bytes as the plain IPv4
// field. Which of the two cases below matches is therefore immaterial for
// any well-formed record; net.IP.Equal compares either form correctly
// against a plain IPv4 address.
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
