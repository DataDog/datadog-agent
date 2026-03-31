// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package codegen

import "fmt"

// This file contains stub implementations of codegen functions that are
// not yet fully implemented. They will be replaced with real implementations
// in subsequent tasks. Each stub sets cs.Err and returns nil.

func stub(cs *CompilerState, name string) {
	cs.SetError(fmt.Errorf("%s: not yet implemented", name))
}

// --- Entry points (Task 11) ---

// GenScode generates code for a symbolic name (host, net, etc.).
func GenScode(cs *CompilerState, name string, q Qual) *Block {
	stub(cs, "gen_scode")
	return nil
}

// GenEcode generates code for an Ethernet address.
func GenEcode(cs *CompilerState, name string, q Qual) *Block {
	stub(cs, "gen_ecode")
	return nil
}

// GenAcode generates code for an ARCnet address.
func GenAcode(cs *CompilerState, name string, q Qual) *Block {
	stub(cs, "gen_acode")
	return nil
}

// GenMcode generates code for a masked address (host/mask or host/prefixlen).
func GenMcode(cs *CompilerState, addr string, mask string, masklen uint32, q Qual) *Block {
	stub(cs, "gen_mcode")
	return nil
}

// GenMcode6 generates code for an IPv6 address with prefix length.
func GenMcode6(cs *CompilerState, addr string, masklen uint32, q Qual) *Block {
	stub(cs, "gen_mcode6")
	return nil
}

// GenNcode generates code for a numeric value (address, port, etc.).
func GenNcode(cs *CompilerState, name string, v uint32, q Qual) *Block {
	stub(cs, "gen_ncode")
	return nil
}

// --- Encapsulation (Task 11) ---

// GenVlan generates code for VLAN matching.
func GenVlan(cs *CompilerState, id uint32, hasID int) *Block {
	stub(cs, "gen_vlan")
	return nil
}

// GenMpls generates code for MPLS matching.
func GenMpls(cs *CompilerState, label uint32, hasLabel int) *Block {
	stub(cs, "gen_mpls")
	return nil
}

// GenPppoed generates code for PPPoE discovery matching.
func GenPppoed(cs *CompilerState) *Block {
	stub(cs, "gen_pppoed")
	return nil
}

// GenPppoes generates code for PPPoE session matching.
func GenPppoes(cs *CompilerState, id uint32, hasID int) *Block {
	stub(cs, "gen_pppoes")
	return nil
}

// GenGeneve generates code for Geneve matching.
func GenGeneve(cs *CompilerState, vni uint32, hasVNI int) *Block {
	stub(cs, "gen_geneve")
	return nil
}

// --- LLC (Task 11) ---

// GenLLC generates code for LLC matching.
func GenLLC(cs *CompilerState) *Block {
	stub(cs, "gen_llc")
	return nil
}

// GenLLCI generates code for LLC I-frame matching.
func GenLLCI(cs *CompilerState) *Block {
	stub(cs, "gen_llc_i")
	return nil
}

// GenLLCS generates code for LLC S-frame matching.
func GenLLCS(cs *CompilerState) *Block {
	stub(cs, "gen_llc_s")
	return nil
}

// GenLLCU generates code for LLC U-frame matching.
func GenLLCU(cs *CompilerState) *Block {
	stub(cs, "gen_llc_u")
	return nil
}

// GenLLCSSubtype generates code for LLC S-frame subtype matching.
func GenLLCSSubtype(cs *CompilerState, subtype uint32) *Block {
	stub(cs, "gen_llc_s_subtype")
	return nil
}

// GenLLCUSubtype generates code for LLC U-frame subtype matching.
func GenLLCUSubtype(cs *CompilerState, subtype uint32) *Block {
	stub(cs, "gen_llc_u_subtype")
	return nil
}

// --- ATM/MTP/PF/802.11 (Task 11 — rare protocols) ---

// GenAtmtypeAbbrev generates code for ATM type matching.
func GenAtmtypeAbbrev(cs *CompilerState, atmtype int) *Block {
	stub(cs, "gen_atmtype_abbrev")
	return nil
}

// GenAtmmultiAbbrev generates code for ATM multi-type matching.
func GenAtmmultiAbbrev(cs *CompilerState, atmtype int) *Block {
	stub(cs, "gen_atmmulti_abbrev")
	return nil
}

// GenAtmfieldCode generates code for ATM field matching.
func GenAtmfieldCode(cs *CompilerState, fieldtype int, val uint32, op int, reversed int) *Block {
	stub(cs, "gen_atmfield_code")
	return nil
}

// GenMtp2typeAbbrev generates code for MTP2 type matching.
func GenMtp2typeAbbrev(cs *CompilerState, mtp2type int) *Block {
	stub(cs, "gen_mtp2type_abbrev")
	return nil
}

// GenMtp3fieldCode generates code for MTP3 field matching.
func GenMtp3fieldCode(cs *CompilerState, fieldtype int, val uint32, op int, reversed int) *Block {
	stub(cs, "gen_mtp3field_code")
	return nil
}

// GenPfIfname generates code for PF ifname matching.
func GenPfIfname(cs *CompilerState, name string) *Block {
	stub(cs, "gen_pf_ifname")
	return nil
}

// GenPfRuleset generates code for PF ruleset matching.
func GenPfRuleset(cs *CompilerState, name string) *Block {
	stub(cs, "gen_pf_ruleset")
	return nil
}

// GenPfRnr generates code for PF rule number matching.
func GenPfRnr(cs *CompilerState, n int) *Block {
	stub(cs, "gen_pf_rnr")
	return nil
}

// GenPfSrnr generates code for PF subrule number matching.
func GenPfSrnr(cs *CompilerState, n int) *Block {
	stub(cs, "gen_pf_srnr")
	return nil
}

// GenPfReason generates code for PF reason matching.
func GenPfReason(cs *CompilerState, reason int) *Block {
	stub(cs, "gen_pf_reason")
	return nil
}

// GenPfAction generates code for PF action matching.
func GenPfAction(cs *CompilerState, action int) *Block {
	stub(cs, "gen_pf_action")
	return nil
}

// GenP80211Type generates code for IEEE 802.11 type matching.
func GenP80211Type(cs *CompilerState, typeval uint32, mask uint32) *Block {
	stub(cs, "gen_p80211_type")
	return nil
}

// GenP80211Fcdir generates code for IEEE 802.11 direction matching.
func GenP80211Fcdir(cs *CompilerState, dir uint32) *Block {
	stub(cs, "gen_p80211_fcdir")
	return nil
}
