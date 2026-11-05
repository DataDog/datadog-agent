// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package portlist

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"strconv"

	"golang.org/x/sys/unix"
)

const (
	tcpStateListen   = 10 // TCP_LISTEN in include/net/tcp_states.h
	nlmsgHdrLen      = 16 // sizeof(struct nlmsghdr)
	inetDiagReqV2Len = 56 // sizeof(struct inet_diag_req_v2)
	inetDiagMsgLen   = 72 // sizeof(struct inet_diag_msg)
	sockDiagRecvBuf  = 32 << 10
	// Mirrors the per-file row cap of the /proc/net parser.
	maxSockDiagListeners = 1e6
)

var (
	v4Loopback = netip.AddrFrom4([4]byte{127, 0, 0, 1})
	v6Loopback = netip.IPv6Loopback()
)

// sockDiagListener is one TCP listening socket from a sock_diag dump.
type sockDiagListener struct {
	ip    netip.Addr // v4-mapped IPv6 addresses are unmapped, as in parseHexIPv6
	port  uint16
	inode uint32
	// loopback matches the /proc/net localhost filter: 127.0.0.1 from the
	// IPv4 dump or ::1 from the IPv6 dump.
	loopback bool
}

// dumpTCPListeners is a variable so tests can force the /proc/net fallback.
var dumpTCPListeners = sockDiagTCPListeners

// appendSockDiagListeners marks or adds every TCP listener from a sock_diag
// dump in li.known. On error li.known is left untouched so the caller can
// fall back to /proc/net.
func (li *linuxImpl) appendSockDiagListeners() error {
	ls, err := dumpTCPListeners()
	if err != nil {
		return err
	}
	for _, l := range ls {
		if !li.includeLocalhost && l.loopback {
			continue
		}
		key := append(li.inoBuf[:0], "socket:["...)
		key = strconv.AppendUint(key, uint64(l.inode), 10)
		key = append(key, ']')
		li.inoBuf = key
		if pm, ok := li.known[string(key)]; ok {
			pm.keep = true
			continue
		}
		li.addKnown(string(key), "tcp", l.ip, l.port)
	}
	return nil
}

// sockDiagTCPListeners returns the TCP listeners in the caller's network
// namespace using NETLINK_SOCK_DIAG with idiag_states = TCPF_LISTEN, which
// walks only the listening hash rather than every established socket. It
// opens and closes its own socket, so it is safe to call concurrently.
func sockDiagTCPListeners() ([]sockDiagListener, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_RAW|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, fmt.Errorf("netlink socket: %w", err)
	}
	defer unix.Close(fd)

	// A dump never blocks indefinitely, but don't let a kernel oddity hang the poller.
	tv := unix.Timeval{Sec: 5}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv); err != nil {
		return nil, fmt.Errorf("setting SO_RCVTIMEO: %w", err)
	}

	var out []sockDiagListener
	rbuf := make([]byte, sockDiagRecvBuf)
	for i, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		seq := uint32(i + 1)
		if err := unix.Sendto(fd, inetDiagListenRequest(family, seq), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
			return nil, fmt.Errorf("sending sock_diag request: %w", err)
		}
		out, err = recvInetDiag(fd, rbuf, family, seq, out)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func inetDiagListenRequest(family uint8, seq uint32) []byte {
	b := make([]byte, nlmsgHdrLen+inetDiagReqV2Len)
	ne := binary.NativeEndian
	ne.PutUint32(b[0:4], uint32(len(b)))
	ne.PutUint16(b[4:6], unix.SOCK_DIAG_BY_FAMILY)
	ne.PutUint16(b[6:8], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	ne.PutUint32(b[8:12], seq)
	// inet_diag_req_v2: family, protocol, ext, pad, states, then a zeroed id.
	r := b[nlmsgHdrLen:]
	r[0] = family
	r[1] = unix.IPPROTO_TCP
	ne.PutUint32(r[4:8], 1<<tcpStateListen)
	return b
}

var errSockDiagTruncated = errors.New("sock_diag reply truncated")

// recvInetDiag reads one multipart dump reply until NLMSG_DONE.
func recvInetDiag(fd int, rbuf []byte, family uint8, seq uint32, out []sockDiagListener) ([]sockDiagListener, error) {
	ne := binary.NativeEndian
	count := 0
	for {
		n, _, rflags, _, err := unix.Recvmsg(fd, rbuf, nil, 0)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("receiving sock_diag reply: %w", err)
		}
		if rflags&unix.MSG_TRUNC != 0 {
			return nil, errSockDiagTruncated
		}
		b := rbuf[:n]
		for len(b) > 0 {
			if len(b) < nlmsgHdrLen {
				return nil, fmt.Errorf("short netlink header (%d bytes)", len(b))
			}
			mlen := int(ne.Uint32(b[0:4]))
			typ := ne.Uint16(b[4:6])
			flags := ne.Uint16(b[6:8])
			if mlen < nlmsgHdrLen || mlen > len(b) {
				return nil, fmt.Errorf("bad netlink message length %d", mlen)
			}
			if got := ne.Uint32(b[8:12]); got != seq {
				return nil, fmt.Errorf("unexpected netlink seq %d, want %d", got, seq)
			}
			if flags&unix.NLM_F_DUMP_INTR != 0 {
				return nil, errors.New("sock_diag dump interrupted")
			}
			payload := b[nlmsgHdrLen:mlen]

			switch typ {
			case unix.NLMSG_DONE:
				if len(payload) >= 4 {
					if e := int32(ne.Uint32(payload[0:4])); e < 0 {
						return nil, fmt.Errorf("sock_diag dump: %w", unix.Errno(-e))
					}
				}
				return out, nil
			case unix.NLMSG_ERROR:
				if len(payload) < 4 {
					return nil, errors.New("short NLMSG_ERROR")
				}
				return nil, fmt.Errorf("sock_diag request: %w", unix.Errno(-int32(ne.Uint32(payload[0:4]))))
			case unix.SOCK_DIAG_BY_FAMILY:
				l, ok, err := parseInetDiagMsg(payload, family)
				if err != nil {
					return nil, err
				}
				if ok && count < maxSockDiagListeners {
					out = append(out, l)
					count++
				}
			default:
				return nil, fmt.Errorf("unexpected netlink message type %d", typ)
			}

			adv := (mlen + unix.NLMSG_ALIGNTO - 1) &^ (unix.NLMSG_ALIGNTO - 1)
			if adv > len(b) {
				adv = len(b)
			}
			b = b[adv:]
		}
	}
}

// parseInetDiagMsg decodes a struct inet_diag_msg. ok is false for entries
// that aren't TCP listeners of the requested family.
func parseInetDiagMsg(m []byte, family uint8) (l sockDiagListener, ok bool, err error) {
	if len(m) < inetDiagMsgLen {
		return l, false, fmt.Errorf("short inet_diag_msg (%d bytes)", len(m))
	}
	if m[0] != family || m[1] != tcpStateListen {
		return l, false, nil
	}
	// inet_diag_sockid: sport and dport are big-endian, src at offset 8.
	l.port = binary.BigEndian.Uint16(m[4:6])
	l.inode = binary.NativeEndian.Uint32(m[68:72])
	if family == unix.AF_INET {
		l.ip = netip.AddrFrom4([4]byte(m[8:12]))
		l.loopback = l.ip == v4Loopback
	} else {
		ip := netip.AddrFrom16([16]byte(m[8:24]))
		l.loopback = ip == v6Loopback
		l.ip = ip.Unmap()
	}
	return l, true, nil
}
