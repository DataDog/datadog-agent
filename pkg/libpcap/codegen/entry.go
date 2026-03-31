// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// GenScode generates code for a symbolic name (host, net, port, etc.).
// This is the main entry point called by the grammar for ID tokens.
// Port of gen_scode() from gencode.c.
func GenScode(cs *CompilerState, name string, q Qual) *Block {
	proto := int(q.Proto)
	dir := int(q.Dir)

	switch q.Addr {
	case QNet:
		return genScodeNet(cs, name, proto, dir)

	case QDefault, QHost:
		return genScodeHost(cs, name, proto, dir, q.Addr)

	case QPort:
		return genScodePort(cs, name, proto, dir)

	case QPortrange:
		return genScodePortrange(cs, name, proto, dir)

	case QProto:
		return genScodeProto(cs, name, proto, dir)

	case QGateway:
		cs.SetError(fmt.Errorf("'gateway' not supported"))
		return nil

	default:
		cs.SetError(fmt.Errorf("invalid address qualifier %d", q.Addr))
		return nil
	}
}

func genScodeNet(cs *CompilerState, name string, proto, dir int) *Block {
	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}
	addr, mask, err := cs.Resolver.LookupNet(name)
	if err != nil {
		cs.SetError(fmt.Errorf("unknown network '%s'", name))
		return nil
	}
	return GenHost(cs, addr, mask, proto, dir, QNet)
}

func genScodeHost(cs *CompilerState, name string, proto, dir int, addrQual uint8) *Block {
	if proto == QLink {
		return genScodeEtherHost(cs, name, dir)
	}

	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}

	// Try IPv4
	var b, tmp *Block
	tproto := proto
	tproto6 := proto

	addrs, err := cs.Resolver.LookupHost(name)
	if err == nil {
		for _, addr := range addrs {
			if tproto == QIPv6 {
				continue // skip IPv4 when looking for IPv6
			}
			tmp = GenHost(cs, addr, 0xffffffff, tproto, dir, int(addrQual))
			if b != nil && tmp != nil {
				GenOr(b, tmp)
			}
			b = tmp
		}
	}

	// Try IPv6
	addrs6, err6 := cs.Resolver.LookupHost6(name)
	if err6 == nil {
		if tproto6 == QARP || tproto6 == QIP || tproto6 == QRARP {
			// Skip IPv6 for ARP/IP/RARP-specific queries
		} else {
			var mask128 [16]byte
			for i := range mask128 {
				mask128[i] = 0xff
			}
			for _, addr := range addrs6 {
				tmp = GenHost6(cs, addr, mask128, tproto6, dir, int(addrQual))
				if b != nil && tmp != nil {
					GenOr(b, tmp)
				}
				b = tmp
			}
		}
	}

	if b == nil {
		cs.SetError(fmt.Errorf("unknown host '%s'", name))
	}
	return b
}

func genScodeEtherHost(cs *CompilerState, name string, dir int) *Block {
	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}
	eaddr, err := cs.Resolver.LookupEther(name)
	if err != nil {
		cs.SetError(fmt.Errorf("unknown ether host '%s'", name))
		return nil
	}
	switch cs.Linktype {
	case DLTEN10MB:
		return GenEhostop(cs, eaddr, dir)
	default:
		cs.SetError(fmt.Errorf("link-level host name not supported for DLT %d", cs.Linktype))
		return nil
	}
}

func genScodePort(cs *CompilerState, name string, proto, dir int) *Block {
	if proto != QDefault && proto != QUDP && proto != QTCP && proto != QSCTP {
		cs.SetError(fmt.Errorf("illegal qualifier of 'port'"))
		return nil
	}

	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}

	realProto := ProtoUndef
	port, err := cs.Resolver.LookupPort(name, protoQualToIPProto(proto))
	if err != nil {
		cs.SetError(fmt.Errorf("unknown port '%s'", name))
		return nil
	}

	if proto == QUDP {
		realProto = IPProtoUDP
	} else if proto == QTCP {
		realProto = IPProtoTCP
	} else if proto == QSCTP {
		realProto = IPProtoSCTP
	}

	b := GenPort(cs, uint32(port), realProto, dir)
	b6 := GenPort6(cs, uint32(port), realProto, dir)
	if b != nil && b6 != nil {
		GenOr(b6, b)
	}
	return b
}

func genScodePortrange(cs *CompilerState, name string, proto, dir int) *Block {
	if proto != QDefault && proto != QUDP && proto != QTCP && proto != QSCTP {
		cs.SetError(fmt.Errorf("illegal qualifier of 'portrange'"))
		return nil
	}

	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}

	realProto := ProtoUndef
	p1, p2, err := cs.Resolver.LookupPortRange(name, protoQualToIPProto(proto))
	if err != nil {
		cs.SetError(fmt.Errorf("unknown port range '%s'", name))
		return nil
	}

	if proto == QUDP {
		realProto = IPProtoUDP
	} else if proto == QTCP {
		realProto = IPProtoTCP
	} else if proto == QSCTP {
		realProto = IPProtoSCTP
	}

	b := GenPortrange(cs, uint32(p1), uint32(p2), realProto, dir)
	b6 := GenPortrange6(cs, uint32(p1), uint32(p2), realProto, dir)
	if b != nil && b6 != nil {
		GenOr(b6, b)
	}
	return b
}

func genScodeProto(cs *CompilerState, name string, proto, dir int) *Block {
	if cs.Resolver == nil {
		cs.SetError(fmt.Errorf("name resolution not available"))
		return nil
	}
	realProto, err := cs.Resolver.LookupProto(name)
	if err != nil {
		cs.SetError(fmt.Errorf("unknown protocol: %s", name))
		return nil
	}
	return genProto(cs, uint32(realProto), proto, dir)
}

// GenNcode generates code for a numeric value (address, port, protocol number).
// Port of gen_ncode() from gencode.c.
func GenNcode(cs *CompilerState, s string, v uint32, q Qual) *Block {
	proto := int(q.Proto)
	dir := int(q.Dir)

	var vlen int
	if s == "" {
		vlen = 32
	} else {
		var err error
		vlen, v, err = parseIPv4Addr(s)
		if err != nil {
			cs.SetError(fmt.Errorf("invalid IPv4 address '%s'", s))
			return nil
		}
	}

	switch q.Addr {
	case QDefault, QHost, QNet:
		if proto == QLink {
			cs.SetError(fmt.Errorf("illegal link layer address"))
			return nil
		}
		mask := uint32(0xffffffff)
		if s == "" && q.Addr == QNet {
			// Promote short net number
			for v != 0 && (v&0xff000000) == 0 {
				v <<= 8
				mask <<= 8
			}
		} else {
			// Promote short ipaddr
			v <<= uint(32 - vlen)
			mask <<= uint(32 - vlen)
		}
		return GenHost(cs, v, mask, proto, dir, int(q.Addr))

	case QPort:
		ipProto := protoQualToIPProto(proto)
		if proto != QDefault && proto != QUDP && proto != QTCP && proto != QSCTP {
			cs.SetError(fmt.Errorf("illegal qualifier of 'port'"))
			return nil
		}
		if v > 65535 {
			cs.SetError(fmt.Errorf("illegal port number %d > 65535", v))
			return nil
		}
		b := GenPort(cs, v, ipProto, dir)
		b6 := GenPort6(cs, v, ipProto, dir)
		if b != nil && b6 != nil {
			GenOr(b6, b)
		}
		return b

	case QPortrange:
		ipProto := protoQualToIPProto(proto)
		if proto != QDefault && proto != QUDP && proto != QTCP && proto != QSCTP {
			cs.SetError(fmt.Errorf("illegal qualifier of 'portrange'"))
			return nil
		}
		if v > 65535 {
			cs.SetError(fmt.Errorf("illegal port number %d > 65535", v))
			return nil
		}
		b := GenPortrange(cs, v, v, ipProto, dir)
		b6 := GenPortrange6(cs, v, v, ipProto, dir)
		if b != nil && b6 != nil {
			GenOr(b6, b)
		}
		return b

	case QProto:
		return genProto(cs, v, proto, dir)

	case QGateway:
		cs.SetError(fmt.Errorf("'gateway' requires a name"))
		return nil

	default:
		cs.SetError(fmt.Errorf("invalid address qualifier %d", q.Addr))
		return nil
	}
}

// GenMcode generates code for a masked IPv4 address (addr/mask or addr/prefixlen).
// Port of gen_mcode() from gencode.c.
func GenMcode(cs *CompilerState, s1, s2 string, masklen uint32, q Qual) *Block {
	nlen, n, err := parseIPv4Addr(s1)
	if err != nil {
		cs.SetError(fmt.Errorf("invalid IPv4 address '%s'", s1))
		return nil
	}
	n <<= uint(32 - nlen)

	var m uint32
	if s2 != "" {
		mlen, mval, err := parseIPv4Addr(s2)
		if err != nil {
			cs.SetError(fmt.Errorf("invalid IPv4 address '%s'", s2))
			return nil
		}
		m = mval << uint(32-mlen)
	} else {
		if masklen > 32 {
			cs.SetError(fmt.Errorf("mask length must be <= 32"))
			return nil
		}
		if masklen == 0 {
			m = 0
		} else {
			m = 0xffffffff << (32 - masklen)
		}
	}

	if n&^m != 0 {
		cs.SetError(fmt.Errorf("non-network bits set in \"%s\"", s1))
		return nil
	}

	switch q.Addr {
	case QNet:
		return GenHost(cs, n, m, int(q.Proto), int(q.Dir), QNet)
	default:
		cs.SetError(fmt.Errorf("mask syntax for networks only"))
		return nil
	}
}

// GenMcode6 generates code for an IPv6 address with prefix length.
// Port of gen_mcode6() from gencode.c.
func GenMcode6(cs *CompilerState, s string, masklen uint32, q Qual) *Block {
	ip := net.ParseIP(s)
	if ip == nil {
		cs.SetError(fmt.Errorf("invalid IPv6 address '%s'", s))
		return nil
	}
	v6 := ip.To16()
	if v6 == nil {
		cs.SetError(fmt.Errorf("not an IPv6 address '%s'", s))
		return nil
	}

	var addr [16]byte
	copy(addr[:], v6)

	// Build prefix mask
	var mask [16]byte
	remaining := masklen
	for i := 0; i < 16; i++ {
		if remaining >= 8 {
			mask[i] = 0xff
			remaining -= 8
		} else if remaining > 0 {
			mask[i] = byte(0xff << (8 - remaining))
			remaining = 0
		}
	}

	switch q.Addr {
	case QDefault, QHost, QNet:
		return GenHost6(cs, addr, mask, int(q.Proto), int(q.Dir), int(q.Addr))
	default:
		cs.SetError(fmt.Errorf("invalid address qualifier for IPv6"))
		return nil
	}
}

// GenEcode generates code for an Ethernet MAC address.
// Port of gen_ecode() from gencode.c.
func GenEcode(cs *CompilerState, s string, q Qual) *Block {
	hw, err := net.ParseMAC(s)
	if err != nil {
		cs.SetError(fmt.Errorf("invalid MAC address '%s'", s))
		return nil
	}
	if len(hw) != 6 {
		cs.SetError(fmt.Errorf("MAC address must be 6 bytes, got %d", len(hw)))
		return nil
	}

	if q.Addr != QDefault && q.Addr != QHost {
		cs.SetError(fmt.Errorf("ethernet address used in non-host context"))
		return nil
	}
	if q.Proto != QLink && q.Proto != QDefault {
		cs.SetError(fmt.Errorf("ethernet address used with non-link protocol"))
		return nil
	}

	switch cs.Linktype {
	case DLTEN10MB:
		return GenEhostop(cs, hw, int(q.Dir))
	default:
		cs.SetError(fmt.Errorf("ethernet addresses not supported for DLT %d", cs.Linktype))
		return nil
	}
}

// GenAcode generates code for an ARCnet address.
// Port of gen_acode() from gencode.c.
func GenAcode(cs *CompilerState, s string, q Qual) *Block {
	cs.SetError(fmt.Errorf("ARCnet address matching not supported"))
	return nil
}

// parseIPv4Addr parses a dotted-decimal IPv4 address and returns
// the number of bits, the value, and any error.
// Handles partial addresses: "10" → 8 bits, "10.1" → 16 bits, etc.
func parseIPv4Addr(s string) (int, uint32, error) {
	parts := strings.Split(s, ".")
	if len(parts) > 4 {
		return 0, 0, fmt.Errorf("invalid IPv4 address")
	}

	var v uint32
	bits := 0
	for _, part := range parts {
		n, err := strconv.ParseUint(part, 0, 32)
		if err != nil {
			return 0, 0, err
		}
		if n > 255 {
			return 0, 0, fmt.Errorf("octet %d > 255", n)
		}
		v = (v << 8) | uint32(n)
		bits += 8
	}
	return bits, v, nil
}

// protoQualToIPProto converts a protocol qualifier (Q_TCP, Q_UDP, etc.)
// to an IP protocol number, or ProtoUndef for Q_DEFAULT.
func protoQualToIPProto(proto int) int {
	switch proto {
	case QTCP:
		return IPProtoTCP
	case QUDP:
		return IPProtoUDP
	case QSCTP:
		return IPProtoSCTP
	default:
		return ProtoUndef
	}
}

// Ensure nameresolver types match codegen.NameResolver interface
// by adding the LookupPortRange method signature check
var _ NameResolver = (*nameResolverCheck)(nil)

type nameResolverCheck struct{}

func (n *nameResolverCheck) LookupHost(name string) ([]uint32, error)            { return nil, nil }
func (n *nameResolverCheck) LookupHost6(name string) ([][16]byte, error)          { return nil, nil }
func (n *nameResolverCheck) LookupPort(name string, proto int) (int, error)       { return 0, nil }
func (n *nameResolverCheck) LookupProto(name string) (int, error)                 { return 0, nil }
func (n *nameResolverCheck) LookupEProto(name string) (int, error)                { return 0, nil }
func (n *nameResolverCheck) LookupLLC(name string) (int, error)                   { return 0, nil }
func (n *nameResolverCheck) LookupNet(name string) (uint32, uint32, error)        { return 0, 0, nil }
func (n *nameResolverCheck) LookupEther(name string) ([]byte, error)              { return nil, nil }
func (n *nameResolverCheck) LookupPortRange(name string, proto int) (int, int, error) {
	return 0, 0, nil
}

// Ensure nameresolver.Resolver satisfies codegen.NameResolver.
// This is verified at build time by the nameresolver package importing
// and using this interface. We add a helper to convert for convenience.

// ParseIPv4 is a helper that parses an IPv4 address string and returns
// the address as a uint32 in network byte order.
func ParseIPv4(s string) (uint32, error) {
	ip := net.ParseIP(s)
	if ip == nil {
		return 0, fmt.Errorf("invalid IP address: %s", s)
	}
	v4 := ip.To4()
	if v4 == nil {
		return 0, fmt.Errorf("not an IPv4 address: %s", s)
	}
	return binary.BigEndian.Uint32(v4), nil
}
