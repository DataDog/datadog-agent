// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// GenPort generates code to match a TCP/UDP/SCTP port on IPv4.
// ipProto is IPPROTO_TCP, IPPROTO_UDP, IPPROTO_SCTP, or ProtoUndef (check all three).
// Port of gen_port() from gencode.c.
func GenPort(cs *CompilerState, port uint32, ipProto int, dir int) *Block {
	// Check ethertype is IP
	b0 := GenLinktype(cs, EthertypeIP)
	if b0 == nil {
		return nil
	}

	var b1 *Block
	switch ipProto {
	case IPProtoTCP, IPProtoUDP, IPProtoSCTP:
		b1 = genPortop(cs, port, uint32(ipProto), dir)
	case ProtoUndef:
		// Check TCP, UDP, and SCTP
		tmp := genPortop(cs, port, IPProtoTCP, dir)
		b1 = genPortop(cs, port, IPProtoUDP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
		tmp = genPortop(cs, port, IPProtoSCTP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %d for port matching", ipProto))
		return nil
	}

	if b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// GenPort6 generates code to match a TCP/UDP/SCTP port on IPv6.
// Port of gen_port6() from gencode.c.
func GenPort6(cs *CompilerState, port uint32, ipProto int, dir int) *Block {
	b0 := GenLinktype(cs, EthertypeIPv6)
	if b0 == nil {
		return nil
	}

	var b1 *Block
	switch ipProto {
	case IPProtoTCP, IPProtoUDP, IPProtoSCTP:
		b1 = genPortop6(cs, port, uint32(ipProto), dir)
	case ProtoUndef:
		tmp := genPortop6(cs, port, IPProtoTCP, dir)
		b1 = genPortop6(cs, port, IPProtoUDP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
		tmp = genPortop6(cs, port, IPProtoSCTP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %d for port matching", ipProto))
		return nil
	}

	if b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortop generates code to check IP protocol and port.
// Checks: ip proto == proto AND not-a-fragment AND port matches.
// Port of gen_portop() from gencode.c.
func genPortop(cs *CompilerState, port, proto uint32, dir int) *Block {
	// Check IP protocol field (byte 9 of IP header)
	tmp := GenCmp(cs, OrLinkpl, 9, bpf.BPF_B, proto)
	// Check not a non-first fragment
	b0 := genIPfrag(cs)
	if tmp != nil && b0 != nil {
		GenAnd(tmp, b0)
	}

	var b1 *Block
	switch dir {
	case QSrc:
		b1 = genPortatom(cs, 0, port)
	case QDst:
		b1 = genPortatom(cs, 2, port)
	case QAnd:
		tmp = genPortatom(cs, 0, port)
		b1 = genPortatom(cs, 2, port)
		if tmp != nil && b1 != nil {
			GenAnd(tmp, b1)
		}
	case QDefault, QOr:
		tmp = genPortatom(cs, 0, port)
		b1 = genPortatom(cs, 2, port)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("invalid direction %d for port", dir))
		return nil
	}

	if b0 != nil && b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortop6 generates code to check IPv6 next-header and port.
// Port of gen_portop6() from gencode.c.
func genPortop6(cs *CompilerState, port, proto uint32, dir int) *Block {
	// Check IPv6 next-header field (byte 6 of IPv6 header)
	b0 := GenCmp(cs, OrLinkpl, 6, bpf.BPF_B, proto)

	var b1 *Block
	switch dir {
	case QSrc:
		b1 = genPortatom6(cs, 0, port)
	case QDst:
		b1 = genPortatom6(cs, 2, port)
	case QAnd:
		tmp := genPortatom6(cs, 0, port)
		b1 = genPortatom6(cs, 2, port)
		if tmp != nil && b1 != nil {
			GenAnd(tmp, b1)
		}
	case QDefault, QOr:
		tmp := genPortatom6(cs, 0, port)
		b1 = genPortatom6(cs, 2, port)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("invalid direction %d for port6", dir))
		return nil
	}

	if b0 != nil && b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortatom generates a comparison to a port value in the IPv4 transport header.
func genPortatom(cs *CompilerState, off uint32, port uint32) *Block {
	return GenCmp(cs, OrTranIPv4, off, bpf.BPF_H, port)
}

// genPortatom6 generates a comparison to a port value in the IPv6 transport header.
func genPortatom6(cs *CompilerState, off uint32, port uint32) *Block {
	return GenCmp(cs, OrTranIPv6, off, bpf.BPF_H, port)
}

// genIPfrag generates code to check that a packet is not a non-first IP fragment.
// Checks: (IP flags+fragoffset & 0x1fff) == 0
// Port of gen_ipfrag() from gencode.c.
func genIPfrag(cs *CompilerState) *Block {
	s := GenLoadA(cs, OrLinkpl, 6, bpf.BPF_H)
	b := cs.NewBlock(JmpCode(int(bpf.BPF_JSET)), 0x1fff)
	b.Stmts = s
	GenNot(b)
	return b
}

// GenPortrange generates code to match a port range on IPv4.
// Port of gen_portrange() from gencode.c.
func GenPortrange(cs *CompilerState, port1, port2 uint32, ipProto int, dir int) *Block {
	b0 := GenLinktype(cs, EthertypeIP)
	if b0 == nil {
		return nil
	}

	var b1 *Block
	switch ipProto {
	case IPProtoTCP, IPProtoUDP, IPProtoSCTP:
		b1 = genPortrangeop(cs, port1, port2, uint32(ipProto), dir)
	case ProtoUndef:
		tmp := genPortrangeop(cs, port1, port2, IPProtoTCP, dir)
		b1 = genPortrangeop(cs, port1, port2, IPProtoUDP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
		tmp = genPortrangeop(cs, port1, port2, IPProtoSCTP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %d for portrange", ipProto))
		return nil
	}

	if b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// GenPortrange6 generates code to match a port range on IPv6.
func GenPortrange6(cs *CompilerState, port1, port2 uint32, ipProto int, dir int) *Block {
	b0 := GenLinktype(cs, EthertypeIPv6)
	if b0 == nil {
		return nil
	}

	var b1 *Block
	switch ipProto {
	case IPProtoTCP, IPProtoUDP, IPProtoSCTP:
		b1 = genPortrangeop6(cs, port1, port2, uint32(ipProto), dir)
	case ProtoUndef:
		tmp := genPortrangeop6(cs, port1, port2, IPProtoTCP, dir)
		b1 = genPortrangeop6(cs, port1, port2, IPProtoUDP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
		tmp = genPortrangeop6(cs, port1, port2, IPProtoSCTP, dir)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %d for portrange6", ipProto))
		return nil
	}

	if b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortrangeop generates code to match a port range with protocol check and frag check.
func genPortrangeop(cs *CompilerState, port1, port2, proto uint32, dir int) *Block {
	tmp := GenCmp(cs, OrLinkpl, 9, bpf.BPF_B, proto)
	b0 := genIPfrag(cs)
	if tmp != nil && b0 != nil {
		GenAnd(tmp, b0)
	}

	var b1 *Block
	switch dir {
	case QSrc:
		b1 = genPortrangeatom(cs, OrTranIPv4, 0, port1, port2)
	case QDst:
		b1 = genPortrangeatom(cs, OrTranIPv4, 2, port1, port2)
	case QAnd:
		tmp = genPortrangeatom(cs, OrTranIPv4, 0, port1, port2)
		b1 = genPortrangeatom(cs, OrTranIPv4, 2, port1, port2)
		if tmp != nil && b1 != nil {
			GenAnd(tmp, b1)
		}
	case QDefault, QOr:
		tmp = genPortrangeatom(cs, OrTranIPv4, 0, port1, port2)
		b1 = genPortrangeatom(cs, OrTranIPv4, 2, port1, port2)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("invalid direction %d for portrange", dir))
		return nil
	}

	if b0 != nil && b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortrangeop6 generates port range matching for IPv6.
func genPortrangeop6(cs *CompilerState, port1, port2, proto uint32, dir int) *Block {
	b0 := GenCmp(cs, OrLinkpl, 6, bpf.BPF_B, proto)

	var b1 *Block
	switch dir {
	case QSrc:
		b1 = genPortrangeatom(cs, OrTranIPv6, 0, port1, port2)
	case QDst:
		b1 = genPortrangeatom(cs, OrTranIPv6, 2, port1, port2)
	case QAnd:
		tmp := genPortrangeatom(cs, OrTranIPv6, 0, port1, port2)
		b1 = genPortrangeatom(cs, OrTranIPv6, 2, port1, port2)
		if tmp != nil && b1 != nil {
			GenAnd(tmp, b1)
		}
	case QDefault, QOr:
		tmp := genPortrangeatom(cs, OrTranIPv6, 0, port1, port2)
		b1 = genPortrangeatom(cs, OrTranIPv6, 2, port1, port2)
		if tmp != nil && b1 != nil {
			GenOr(tmp, b1)
		}
	default:
		cs.SetError(fmt.Errorf("invalid direction %d for portrange6", dir))
		return nil
	}

	if b0 != nil && b1 != nil {
		GenAnd(b0, b1)
	}
	return b1
}

// genPortrangeatom generates code to match a port in the range [v1, v2].
// port >= v1 AND port <= v2
func genPortrangeatom(cs *CompilerState, offrel OffsetRel, off, v1, v2 uint32) *Block {
	if v1 > v2 {
		v1, v2 = v2, v1
	}
	b1 := GenCmpGe(cs, offrel, off, bpf.BPF_H, v1)
	b2 := GenCmpLe(cs, offrel, off, bpf.BPF_H, v2)
	if b1 != nil && b2 != nil {
		GenAnd(b1, b2)
	}
	return b2
}
