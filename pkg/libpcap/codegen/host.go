// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"encoding/binary"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// GenHost generates code to match a host or network address.
// addr and mask are in network byte order. proto is the protocol qualifier
// (Q_IP, Q_ARP, Q_DEFAULT, etc.), dir is Q_SRC/Q_DST/Q_OR/Q_AND,
// addrType is Q_HOST or Q_NET.
// Port of gen_host() from gencode.c.
func GenHost(cs *CompilerState, addr, mask uint32, proto, dir, addrType int) *Block {
	typestr := "host"
	if addrType == QNet {
		typestr = "net"
	}

	switch proto {
	case QDefault:
		b0 := GenHost(cs, addr, mask, QIP, dir, addrType)
		if cs.MPLSStackDepth == 0 {
			b1 := GenHost(cs, addr, mask, QARP, dir, addrType)
			if b0 != nil && b1 != nil {
				GenOr(b0, b1)
			}
			b0 = GenHost(cs, addr, mask, QRARP, dir, addrType)
			if b1 != nil && b0 != nil {
				GenOr(b1, b0)
			}
		}
		return b0

	case QIP:
		// IP: check ethertype, then compare src/dst IP address
		// IPv4 src at offset 12, dst at offset 16 (from start of IP header)
		return genHostop(cs, addr, mask, dir, EthertypeIP, 12, 16)

	case QARP:
		// ARP: sender at offset 14, target at offset 24
		return genHostop(cs, addr, mask, dir, EthertypeARP, 14, 24)

	case QRARP:
		// RARP: same offsets as ARP
		return genHostop(cs, addr, mask, dir, EthertypeRevarp, 14, 24)

	case QLink:
		cs.SetError(fmt.Errorf("link-layer modifier applied to %s", typestr))
		return nil
	case QSCTP, QTCP, QUDP, QICMP, QIGMP, QIGRP:
		cs.SetError(fmt.Errorf("'%s' modifier applied to %s", protoName(proto), typestr))
		return nil
	case QIPv6:
		cs.SetError(fmt.Errorf("'ip6' modifier applied to ip host"))
		return nil
	case QICMPv6, QAH, QESP, QPIM, QVRRP, QCARP:
		cs.SetError(fmt.Errorf("'%s' modifier applied to %s", protoName(proto), typestr))
		return nil
	case QAtalk, QAARP, QDecnet, QLat, QSCA, QMoprc, QMopdl:
		cs.SetError(fmt.Errorf("%s host filtering not implemented", protoName(proto)))
		return nil
	case QISO, QESIS, QISIS, QCLNP, QSTP, QIPX, QNetbeui:
		cs.SetError(fmt.Errorf("'%s' modifier applied to %s", protoName(proto), typestr))
		return nil
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %d for host matching", proto))
		return nil
	}
}

// genHostop generates code to match an IPv4 address at a specific layer.
// Checks ethertype, then compares the masked address at the appropriate offset.
// Port of gen_hostop() from gencode.c.
func genHostop(cs *CompilerState, addr, mask uint32, dir int, llProto uint32, srcOff, dstOff uint32) *Block {
	switch dir {
	case QSrc:
		return genHostopSingle(cs, addr, mask, llProto, srcOff)
	case QDst:
		return genHostopSingle(cs, addr, mask, llProto, dstOff)
	case QAnd:
		b0 := genHostop(cs, addr, mask, QSrc, llProto, srcOff, dstOff)
		b1 := genHostop(cs, addr, mask, QDst, llProto, srcOff, dstOff)
		if b0 != nil && b1 != nil {
			GenAnd(b0, b1)
		}
		return b1
	case QDefault, QOr:
		b0 := genHostop(cs, addr, mask, QSrc, llProto, srcOff, dstOff)
		b1 := genHostop(cs, addr, mask, QDst, llProto, srcOff, dstOff)
		if b0 != nil && b1 != nil {
			GenOr(b0, b1)
		}
		return b1
	case QAddr1, QAddr2, QAddr3, QAddr4:
		cs.SetError(fmt.Errorf("'addr' qualifiers are only valid for 802.11 MAC addresses"))
		return nil
	case QRA, QTA:
		cs.SetError(fmt.Errorf("'ra'/'ta' qualifiers are only valid for 802.11 MAC addresses"))
		return nil
	default:
		cs.SetError(fmt.Errorf("unknown direction %d", dir))
		return nil
	}
}

func genHostopSingle(cs *CompilerState, addr, mask uint32, llProto uint32, offset uint32) *Block {
	b0 := GenLinktype(cs, llProto)
	b1 := GenMcmp(cs, OrLinkpl, offset, bpf.BPF_W, addr, mask)
	if b0 != nil && b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// GenHost6 generates code to match an IPv6 host or network address.
// Port of gen_host6() from gencode.c.
func GenHost6(cs *CompilerState, addr [16]byte, mask [16]byte, proto, dir, addrType int) *Block {
	typestr := "host"
	if addrType == QNet {
		typestr = "net"
	}

	switch proto {
	case QDefault, QIPv6:
		// IPv6 src at offset 8, dst at offset 24 (from start of IPv6 header)
		return genHostop6(cs, addr, mask, dir, EthertypeIPv6, 8, 24)
	case QLink:
		cs.SetError(fmt.Errorf("link-layer modifier applied to ip6 %s", typestr))
		return nil
	case QIP:
		cs.SetError(fmt.Errorf("'ip' modifier applied to ip6 %s", typestr))
		return nil
	default:
		cs.SetError(fmt.Errorf("'%s' modifier applied to ip6 %s", protoName(proto), typestr))
		return nil
	}
}

// genHostop6 generates code to match an IPv6 address at a specific layer.
// Compares 4 x 32-bit words with masks.
// Port of gen_hostop6() from gencode.c.
func genHostop6(cs *CompilerState, addr, mask [16]byte, dir int, llProto uint32, srcOff, dstOff uint32) *Block {
	switch dir {
	case QSrc:
		return genHostop6Single(cs, addr, mask, llProto, srcOff)
	case QDst:
		return genHostop6Single(cs, addr, mask, llProto, dstOff)
	case QAnd:
		b0 := genHostop6(cs, addr, mask, QSrc, llProto, srcOff, dstOff)
		b1 := genHostop6(cs, addr, mask, QDst, llProto, srcOff, dstOff)
		if b0 != nil && b1 != nil {
			GenAnd(b0, b1)
		}
		return b1
	case QDefault, QOr:
		b0 := genHostop6(cs, addr, mask, QSrc, llProto, srcOff, dstOff)
		b1 := genHostop6(cs, addr, mask, QDst, llProto, srcOff, dstOff)
		if b0 != nil && b1 != nil {
			GenOr(b0, b1)
		}
		return b1
	default:
		cs.SetError(fmt.Errorf("unsupported direction %d for IPv6 host", dir))
		return nil
	}
}

func genHostop6Single(cs *CompilerState, addr, mask [16]byte, llProto uint32, offset uint32) *Block {
	// Compare 4 x 32-bit words (in network byte order)
	a := [4]uint32{
		binary.BigEndian.Uint32(addr[0:4]),
		binary.BigEndian.Uint32(addr[4:8]),
		binary.BigEndian.Uint32(addr[8:12]),
		binary.BigEndian.Uint32(addr[12:16]),
	}
	m := [4]uint32{
		binary.BigEndian.Uint32(mask[0:4]),
		binary.BigEndian.Uint32(mask[4:8]),
		binary.BigEndian.Uint32(mask[8:12]),
		binary.BigEndian.Uint32(mask[12:16]),
	}

	b1 := GenMcmp(cs, OrLinkpl, offset+12, bpf.BPF_W, a[3], m[3])
	b0 := GenMcmp(cs, OrLinkpl, offset+8, bpf.BPF_W, a[2], m[2])
	GenAnd(b0, b1)
	b0 = GenMcmp(cs, OrLinkpl, offset+4, bpf.BPF_W, a[1], m[1])
	GenAnd(b0, b1)
	b0 = GenMcmp(cs, OrLinkpl, offset+0, bpf.BPF_W, a[0], m[0])
	GenAnd(b0, b1)
	b0 = GenLinktype(cs, llProto)
	GenAnd(b0, b1)
	return b1
}

// GenEhostop generates code to match an Ethernet MAC address.
// Port of gen_ehostop() from gencode.c.
func GenEhostop(cs *CompilerState, eaddr []byte, dir int) *Block {
	if len(eaddr) != 6 {
		cs.SetError(fmt.Errorf("invalid MAC address length %d", len(eaddr)))
		return nil
	}

	switch dir {
	case QSrc:
		// Source MAC is at offset 6 in Ethernet header
		return GenBcmp(cs, OrLinkhdr, 6, eaddr)
	case QDst:
		// Destination MAC is at offset 0
		return GenBcmp(cs, OrLinkhdr, 0, eaddr)
	case QAnd:
		b0 := GenEhostop(cs, eaddr, QSrc)
		b1 := GenEhostop(cs, eaddr, QDst)
		if b0 != nil && b1 != nil {
			GenAnd(b0, b1)
		}
		return b1
	case QDefault, QOr:
		b0 := GenEhostop(cs, eaddr, QSrc)
		b1 := GenEhostop(cs, eaddr, QDst)
		if b0 != nil && b1 != nil {
			GenOr(b0, b1)
		}
		return b1
	case QAddr1, QAddr2, QAddr3, QAddr4:
		cs.SetError(fmt.Errorf("'addr' qualifiers are only supported on 802.11"))
		return nil
	case QRA, QTA:
		cs.SetError(fmt.Errorf("'ra'/'ta' qualifiers are only supported on 802.11"))
		return nil
	default:
		cs.SetError(fmt.Errorf("unknown direction %d for Ethernet host", dir))
		return nil
	}
}

// protoName returns a human-readable name for a protocol qualifier.
func protoName(proto int) string {
	names := map[int]string{
		QLink: "link", QIP: "ip", QARP: "arp", QRARP: "rarp",
		QSCTP: "sctp", QTCP: "tcp", QUDP: "udp", QICMP: "icmp",
		QIGMP: "igmp", QIGRP: "igrp", QPIM: "pim", QVRRP: "vrrp", QCARP: "carp",
		QAtalk: "atalk", QAARP: "aarp", QDecnet: "decnet", QLat: "lat",
		QSCA: "sca", QMoprc: "moprc", QMopdl: "mopdl",
		QIPv6: "ip6", QICMPv6: "icmp6", QAH: "ah", QESP: "esp",
		QISO: "iso", QESIS: "esis", QISIS: "isis", QCLNP: "clnp",
		QSTP: "stp", QIPX: "ipx", QNetbeui: "netbeui", QRadio: "radio",
	}
	if name, ok := names[proto]; ok {
		return name
	}
	return fmt.Sprintf("proto-%d", proto)
}
