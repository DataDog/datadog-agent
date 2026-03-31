// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// DLT constants (data link types) — only the ones we need.
const (
	DLTNull       = 0
	DLTEN10MB     = 1 // Ethernet
	DLTLoop       = 12
	DLTRaw        = 12  // on OpenBSD
	DLTLinuxSLL   = 113 // Linux cooked
	DLTLinuxSLL2  = 276
	DLTRawIPv4    = 228
	DLTRawIPv6    = 229
)

// OffsetNotSet indicates an offset that has not been set.
const OffsetNotSet = -1

// InitLinktype initializes the compiler state for the given link type.
// Sets all offset fields based on the link-layer header format.
// Port of init_linktype() from gencode.c (Ethernet-focused subset).
func InitLinktype(cs *CompilerState) error {
	// Reset all offsets
	cs.OffLinkhdr = AbsOffset{ConstPart: 0, Reg: -1}
	cs.OffPrevlinkhdr = AbsOffset{ConstPart: 0, Reg: -1}
	cs.OffLinktype = AbsOffset{ConstPart: 0, Reg: -1}
	cs.OffLinkpl = AbsOffset{ConstPart: 0, Reg: -1}
	cs.OffLL = AbsOffset{ConstPart: 0, Reg: -1}
	cs.IsGeneve = false
	cs.VLANStackDepth = 0
	cs.MPLSStackDepth = 0

	switch cs.Linktype {
	case DLTEN10MB: // Ethernet
		cs.OffLinktype.ConstPart = 12   // Ethertype field at offset 12
		cs.OffLinkpl.ConstPart = 14     // Link payload after 14-byte Ethernet header
		cs.OffNl = 0                     // Network layer at start of link payload (Ethernet II)
		cs.OffNlNosnap = 3              // For 802.3+802.2: 3 bytes of LLC header

	case DLTNull, DLTLoop: // Loopback
		cs.OffLinktype.ConstPart = 0
		cs.OffLinkpl.ConstPart = 4
		cs.OffNl = 0
		cs.OffNlNosnap = 0

	case DLTLinuxSLL: // Linux cooked capture
		cs.OffLinktype.ConstPart = 14
		cs.OffLinkpl.ConstPart = 16
		cs.OffNl = 0
		cs.OffNlNosnap = 0

	case DLTLinuxSLL2: // Linux cooked capture v2
		cs.OffLinktype.ConstPart = 0
		cs.OffLinkpl.ConstPart = 20
		cs.OffNl = 0
		cs.OffNlNosnap = 0

	case DLTRawIPv4, DLTRawIPv6, 14: // DLT_RAW on some platforms
		// No link-layer header
		cs.OffLinktype.ConstPart = OffsetNotSet
		cs.OffLinkpl.ConstPart = 0
		cs.OffNl = 0
		cs.OffNlNosnap = 0

	default:
		return fmt.Errorf("unknown data link type %d", cs.Linktype)
	}

	return nil
}

// GenLinktype generates code to match a link-layer protocol type.
// ll_proto is either an Ethernet type (> ETHERMTU) or an LLC SAP value (<= ETHERMTU).
// Port of gen_linktype() from gencode.c (Ethernet-focused subset).
func GenLinktype(cs *CompilerState, llProto uint32) *Block {
	// Check for MPLS encapsulation
	if cs.MPLSStackDepth > 0 {
		return genMplsLinktype(cs, llProto)
	}

	switch cs.Linktype {
	case DLTEN10MB:
		var b0 *Block
		if !cs.IsGeneve {
			b0 = genPrevlinkhdrCheck(cs)
		}
		b1 := genEtherLinktype(cs, llProto)
		if b0 != nil {
			GenAnd(b0, b1)
		}
		return b1

	case DLTNull, DLTLoop:
		return genLoopbackLinktype(cs, llProto)

	case DLTLinuxSLL, DLTLinuxSLL2:
		// Simple ethertype comparison at the linktype offset
		return GenCmp(cs, OrLinktype, 0, bpf.BPF_H, llProto)

	default:
		if cs.OffLinktype.ConstPart == OffsetNotSet {
			// Raw IP — no link-layer header to check
			cs.SetError(fmt.Errorf("link-layer type filtering not supported for DLT %d", cs.Linktype))
			return nil
		}
		return GenCmp(cs, OrLinktype, 0, bpf.BPF_H, llProto)
	}
}

// genEtherLinktype generates Ethernet-specific link-type matching.
// Handles Ethernet II ethertypes, 802.2 LLC SAPs, SNAP, and special protocols.
// Port of gen_ether_linktype() from gencode.c.
func genEtherLinktype(cs *CompilerState, llProto uint32) *Block {
	switch llProto {

	case LLCSAPISONs, LLCSAPIP, LLCSAPNetbeui:
		// These always use 802.2 encapsulation.
		// Check that the frame is 802.3 (length <= ETHERMTU), then check DSAP+SSAP.
		b0 := GenCmpGt(cs, OrLinktype, 0, bpf.BPF_H, EtherMTU)
		GenNot(b0)
		b1 := GenCmp(cs, OrLLC, 0, bpf.BPF_H, (llProto<<8)|llProto)
		GenAnd(b0, b1)
		return b1

	case LLCSAPIPX:
		// IPX can be Ethernet_II, Ethernet_802.3, Ethernet_802.2, or Ethernet_SNAP.

		// Check for 802.2 (IPX LSAP) or raw 802.3 (0xFFFF)
		b0 := GenCmp(cs, OrLLC, 0, bpf.BPF_B, LLCSAPIPX)
		b1 := GenCmp(cs, OrLLC, 0, bpf.BPF_H, 0xFFFF)
		GenOr(b0, b1)

		// Check for SNAP with ETHERTYPE_IPX
		b0 = genSnap(cs, 0x000000, EthertypeIPX)
		GenOr(b0, b1)

		// Must be 802.3 frame (length <= ETHERMTU)
		b0 = GenCmpGt(cs, OrLinktype, 0, bpf.BPF_H, EtherMTU)
		GenNot(b0)
		GenAnd(b0, b1)

		// Also check for Ethernet II (ethertype == IPX)
		b0 = GenCmp(cs, OrLinktype, 0, bpf.BPF_H, EthertypeIPX)
		GenOr(b0, b1)
		return b1

	case EthertypeAtalk, EthertypeAARP:
		// AppleTalk can be Ethernet II or 802.2+SNAP.

		// Check for 802.3 frame
		b0 := GenCmpGt(cs, OrLinktype, 0, bpf.BPF_H, EtherMTU)
		GenNot(b0)

		// Check for SNAP encapsulation
		var b1 *Block
		if llProto == EthertypeAtalk {
			b1 = genSnap(cs, 0x080007, EthertypeAtalk)
		} else {
			b1 = genSnap(cs, 0x000000, EthertypeAARP)
		}
		GenAnd(b0, b1)

		// Also check for Ethernet II
		b0 = GenCmp(cs, OrLinktype, 0, bpf.BPF_H, llProto)
		GenOr(b0, b1)
		return b1

	default:
		if llProto <= EtherMTU {
			// LLC SAP value — frame must be 802.3 (length <= ETHERMTU)
			b0 := GenCmpGt(cs, OrLinktype, 0, bpf.BPF_H, EtherMTU)
			GenNot(b0)
			b1 := GenCmp(cs, OrLLC, 2, bpf.BPF_B, llProto)
			GenAnd(b0, b1)
			return b1
		}
		// Standard Ethernet II ethertype comparison
		return GenCmp(cs, OrLinktype, 0, bpf.BPF_H, llProto)
	}
}

// genSnap generates code to match a SNAP header with the given OUI and protocol type.
// Port of gen_snap() from gencode.c.
func genSnap(cs *CompilerState, orgcode uint32, ptype uint32) *Block {
	snapblock := [8]byte{
		LLCSAPSnap,                     // DSAP = SNAP
		LLCSAPSnap,                     // SSAP = SNAP
		0x03,                           // control = UI
		byte(orgcode >> 16),            // OUI byte 0
		byte(orgcode >> 8),             // OUI byte 1
		byte(orgcode),                  // OUI byte 2
		byte(ptype >> 8),               // protocol type high
		byte(ptype),                    // protocol type low
	}
	return GenBcmp(cs, OrLLC, 0, snapblock[:])
}

// genPrevlinkhdrCheck generates checks for the previous link-layer header
// (used for encapsulated protocols like LANE-over-ATM).
// For standard Ethernet, this returns nil.
// Port of gen_prevlinkhdr_check() from gencode.c.
func genPrevlinkhdrCheck(cs *CompilerState) *Block {
	// For standard Ethernet, no previous link header check is needed
	return nil
}

// genLoopbackLinktype generates link-type matching for loopback/null interfaces.
// Port of gen_loopback_linktype() from gencode.c.
func genLoopbackLinktype(cs *CompilerState, llProto uint32) *Block {
	// Loopback uses AF_ values in the link-layer header.
	// Map ethertypes to AF_ values.
	var afVal uint32
	switch llProto {
	case EthertypeIP:
		afVal = 2 // AF_INET
	case EthertypeIPv6:
		// AF_INET6 varies by platform; use common value
		afVal = 24 // BSD
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %#x on loopback", llProto))
		return nil
	}
	return GenCmp(cs, OrLinkhdr, 0, bpf.BPF_W, afVal)
}

// genMplsLinktype generates link-type matching inside MPLS.
// Port of gen_mpls_linktype() from gencode.c.
func genMplsLinktype(cs *CompilerState, llProto uint32) *Block {
	switch llProto {
	case EthertypeIP:
		// Check for IPv4: first nibble of IP header == 4
		b0 := GenCmp(cs, OrLinkpl, 0, bpf.BPF_B, 0x40)
		b1 := GenMcmp(cs, OrLinkpl, 0, bpf.BPF_B, 0x40, 0xf0)
		_ = b0
		return b1
	case EthertypeIPv6:
		// Check for IPv6: first nibble == 6
		return GenMcmp(cs, OrLinkpl, 0, bpf.BPF_B, 0x60, 0xf0)
	default:
		cs.SetError(fmt.Errorf("unsupported protocol %#x in MPLS", llProto))
		return nil
	}
}

// LLCSAPSnap is the SNAP SAP value (0xAA).
const LLCSAPSnap = 0xaa
