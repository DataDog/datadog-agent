// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// ISO/IS-IS protocol constants
const (
	ISO9542ESIS  = 0x82
	ISO10589ISIS = 0x83
	ISO8473CLNP  = 0x81

	// IS-IS PDU types
	ISISL1LanIIH = 15
	ISISL2LanIIH = 16
	ISISPtpIIH   = 17
	ISISL1LSP    = 18
	ISISL2LSP    = 20
	ISISL1CSNP   = 24
	ISISL2CSNP   = 25
	ISISL1PSNP   = 26
	ISISL2PSNP   = 27

	// IPv6 fragment header
	IPProtoFragment = 44
)

// GenProtoAbbrev generates code for a protocol keyword (tcp, udp, ip, etc.).
// Port of gen_proto_abbrev_internal() from gencode.c.
func GenProtoAbbrev(cs *CompilerState, proto int) *Block {
	switch proto {
	case QSCTP:
		return genProto(cs, IPProtoSCTP, QDefault, QDefault)
	case QTCP:
		return genProto(cs, IPProtoTCP, QDefault, QDefault)
	case QUDP:
		return genProto(cs, IPProtoUDP, QDefault, QDefault)
	case QICMP:
		return genProto(cs, IPProtoICMP, QIP, QDefault)
	case QIGMP:
		return genProto(cs, IPProtoIGMP, QIP, QDefault)
	case QIGRP:
		return genProto(cs, IPProtoIGRP, QIP, QDefault)
	case QPIM:
		return genProto(cs, IPProtoPIM, QDefault, QDefault)
	case QVRRP:
		return genProto(cs, IPProtoVRRP, QIP, QDefault)
	case QCARP:
		return genProto(cs, IPProtoCARP, QIP, QDefault)

	case QIP:
		return GenLinktype(cs, EthertypeIP)
	case QARP:
		return GenLinktype(cs, EthertypeARP)
	case QRARP:
		return GenLinktype(cs, EthertypeRevarp)
	case QIPv6:
		return GenLinktype(cs, EthertypeIPv6)

	case QAtalk:
		return GenLinktype(cs, EthertypeAtalk)
	case QAARP:
		return GenLinktype(cs, EthertypeAARP)
	case QDecnet:
		return GenLinktype(cs, EthertypeDN)
	case QSCA:
		return GenLinktype(cs, EthertypeSCA)
	case QLat:
		return GenLinktype(cs, EthertypeLAT)
	case QMopdl:
		return GenLinktype(cs, EthertypeMopdl)
	case QMoprc:
		return GenLinktype(cs, EthertypeMoprc)

	case QICMPv6:
		return genProto(cs, IPProtoICMPv6, QIPv6, QDefault)
	case QAH:
		return genProto(cs, IPProtoAH, QDefault, QDefault)
	case QESP:
		return genProto(cs, IPProtoESP, QDefault, QDefault)

	case QISO:
		return GenLinktype(cs, LLCSAPISONs)
	case QESIS:
		return genProto(cs, ISO9542ESIS, QISO, QDefault)
	case QISIS:
		return genProto(cs, ISO10589ISIS, QISO, QDefault)
	case QCLNP:
		return genProto(cs, ISO8473CLNP, QISO, QDefault)

	case QISISL1:
		return genISISLevel(cs, []uint32{ISISL1LanIIH, ISISPtpIIH, ISISL1LSP, ISISL1CSNP, ISISL1PSNP})
	case QISISL2:
		return genISISLevel(cs, []uint32{ISISL2LanIIH, ISISPtpIIH, ISISL2LSP, ISISL2CSNP, ISISL2PSNP})
	case QISISIIH:
		return genISISLevel(cs, []uint32{ISISL1LanIIH, ISISL2LanIIH, ISISPtpIIH})
	case QISISLSP:
		return genISISLevel(cs, []uint32{ISISL1LSP, ISISL2LSP})
	case QISISSNP:
		return genISISLevel(cs, []uint32{ISISL1CSNP, ISISL2CSNP, ISISL1PSNP, ISISL2PSNP})
	case QISISCSNP:
		return genISISLevel(cs, []uint32{ISISL1CSNP, ISISL2CSNP})
	case QISISPSNP:
		return genISISLevel(cs, []uint32{ISISL1PSNP, ISISL2PSNP})

	case QSTP:
		return GenLinktype(cs, LLCSAP8021D)
	case QIPX:
		return GenLinktype(cs, LLCSAPIPX)
	case QNetbeui:
		return GenLinktype(cs, LLCSAPNetbeui)

	case QLink:
		cs.SetError(errors.New("link layer applied in wrong context"))
		return nil
	case QRadio:
		cs.SetError(errors.New("'radio' is not a valid protocol type"))
		return nil

	default:
		cs.SetError(fmt.Errorf("unknown protocol abbreviation %d", proto))
		return nil
	}
}

// genProto generates code to check for a specific protocol number at the
// appropriate layer. proto specifies the layer (Q_IP, Q_IPV6, Q_DEFAULT, etc.),
// v is the protocol number to check, dir is the direction qualifier.
// Port of gen_proto() from gencode.c.
func genProto(cs *CompilerState, v uint32, proto int, dir int) *Block {
	if dir != QDefault {
		cs.SetError(errors.New("direction applied to 'proto'"))
		return nil
	}

	switch proto {
	case QDefault:
		// Check both IPv4 and IPv6
		b0 := genProto(cs, v, QIP, dir)
		b1 := genProto(cs, v, QIPv6, dir)
		if b0 == nil || b1 == nil {
			return nil
		}
		GenOr(b0, b1)
		return b1

	case QLink:
		return GenLinktype(cs, v)

	case QIP:
		// Check ethertype is IP, then check IP protocol field (byte 9 of IP header)
		b0 := GenLinktype(cs, EthertypeIP)
		b1 := GenCmp(cs, OrLinkpl, 9, bpf.BPF_B, v)
		if b0 == nil || b1 == nil {
			return nil
		}
		GenAnd(b0, b1)
		return b1

	case QIPv6:
		// Check ethertype is IPv6, then check next header field
		// Also handle fragmentation: if next-header is Fragment (44),
		// check the protocol in the fragment header at offset 40
		b0 := GenLinktype(cs, EthertypeIPv6)
		if b0 == nil {
			return nil
		}
		b2 := GenCmp(cs, OrLinkpl, 6, bpf.BPF_B, IPProtoFragment)
		b1 := GenCmp(cs, OrLinkpl, 40, bpf.BPF_B, v)
		GenAnd(b2, b1)
		b2 = GenCmp(cs, OrLinkpl, 6, bpf.BPF_B, v)
		GenOr(b2, b1)
		GenAnd(b0, b1)
		return b1

	case QISO:
		// Check LLC is ISONS, then check NLPID (first byte of network layer payload)
		b0 := GenLinktype(cs, LLCSAPISONs)
		b1 := GenCmp(cs, OrLinkpl, 0, bpf.BPF_B, v)
		if b0 == nil || b1 == nil {
			return nil
		}
		GenAnd(b0, b1)
		return b1

	case QISIS:
		// IS-IS PDU type is at fixed offset in the IS-IS header
		b0 := genProto(cs, ISO10589ISIS, QISO, QDefault)
		// IS-IS PDU type is at offset 4 within the IS-IS PDU
		b1 := GenCmp(cs, OrLinkpl, 4, bpf.BPF_B, v)
		if b0 == nil || b1 == nil {
			return nil
		}
		GenAnd(b0, b1)
		return b1

	case QARP:
		cs.SetError(errors.New("arp does not encapsulate another protocol"))
		return nil
	case QRARP:
		cs.SetError(errors.New("rarp does not encapsulate another protocol"))
		return nil
	case QSCTP:
		cs.SetError(errors.New("'sctp proto' is bogus"))
		return nil
	case QTCP:
		cs.SetError(errors.New("'tcp proto' is bogus"))
		return nil
	case QUDP:
		cs.SetError(errors.New("'udp proto' is bogus"))
		return nil
	case QICMP:
		cs.SetError(errors.New("'icmp proto' is bogus"))
		return nil
	case QIGMP:
		cs.SetError(errors.New("'igmp proto' is bogus"))
		return nil
	case QIGRP:
		cs.SetError(errors.New("'igrp proto' is bogus"))
		return nil
	case QAtalk:
		cs.SetError(errors.New("AppleTalk encapsulation is not specifiable"))
		return nil
	case QDecnet:
		cs.SetError(errors.New("DECNET encapsulation is not specifiable"))
		return nil
	case QLat:
		cs.SetError(errors.New("LAT does not encapsulate another protocol"))
		return nil
	case QSCA:
		cs.SetError(errors.New("SCA does not encapsulate another protocol"))
		return nil
	case QMoprc:
		cs.SetError(errors.New("MOPRC does not encapsulate another protocol"))
		return nil
	case QMopdl:
		cs.SetError(errors.New("MOPDL does not encapsulate another protocol"))
		return nil
	case QICMPv6:
		cs.SetError(errors.New("'icmp6 proto' is bogus"))
		return nil
	case QAH:
		cs.SetError(errors.New("'ah proto' is bogus"))
		return nil
	case QESP:
		cs.SetError(errors.New("'esp proto' is bogus"))
		return nil
	default:
		cs.SetError(fmt.Errorf("unsupported protocol layer %d for 'proto'", proto))
		return nil
	}
}

// genISISLevel generates code matching multiple IS-IS PDU types (ORed together).
func genISISLevel(cs *CompilerState, pdus []uint32) *Block {
	if len(pdus) == 0 {
		return nil
	}
	b1 := genProto(cs, pdus[0], QISIS, QDefault)
	for _, pdu := range pdus[1:] {
		b0 := genProto(cs, pdu, QISIS, QDefault)
		if b0 == nil || b1 == nil {
			return nil
		}
		GenOr(b0, b1)
	}
	return b1
}
