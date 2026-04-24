#!/bin/bash
# convert_grammar.sh — Mechanical conversion of libpcap grammar.y.in to goyacc draft
#
# Usage: ./convert_grammar.sh <libpcap-src>/grammar.y.in > grammar_draft.y
#
# This script performs the MECHANICAL transformations documented in PORTING.md.
# The output is a draft .y file that requires manual review and adjustment for:
#   - Complex C action code (if/else chains, for loops)
#   - Lookup table references (str2tok)
#   - Push/pop qualifier stack placement
#   - Go import paths
#
# The script handles:
#   1. Stripping C prologue and epilogue
#   2. Removing Bison-only directives
#   3. Replacing %union with Go types
#   4. Merging %type/%token for typed tokens
#   5. Replacing C constants with Go package references
#   6. Replacing $<blk>0 with cs.PeekQual()
#   7. Replacing CHECK_PTR_VAL/CHECK_INT_VAL/QSET macros
#   8. Replacing cstate references

set -euo pipefail

INPUT="${1:?Usage: $0 <path-to-grammar.y.in>}"

if [ ! -f "$INPUT" ]; then
    echo "Error: $INPUT not found" >&2
    exit 1
fi

# Phase 1: Extract the grammar rules (between %% markers)
# We'll build the output in stages

cat <<'HEADER'
%{
// Auto-generated draft from libpcap grammar.y.in — requires manual review.
// See PORTING.md for the full list of modifications.

package grammar

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

var _ = bpf.BPF_JEQ
%}

%union {
	i    int
	h    uint32
	s    string
	blk  struct {
		q              codegen.Qual
		b              *codegen.Block
		atmfieldtype   int
		mtp3fieldtype  int
	}
	rblk *codegen.Block
	a    *codegen.Arth
}

HEADER

# Phase 2: Extract %type and %token declarations, transform them
# We read the original and apply transformations

sed -n '/^%type/p; /^%token/p' "$INPUT" | \
    # Remove %type lines for tokens that also have %token (will be merged)
    grep -v '%type.*\bID\b' | \
    grep -v '%type.*\bEID\b' | \
    grep -v '%type.*\bAID\b' | \
    grep -v '%type.*\bHID\b' | \
    grep -v '%type.*\bHID6\b' | \
    grep -v '%type.*\bNUM\b' | \
    # Replace C Q_ constants in %type lines
    sed 's/struct stmt \*//' | \
    # Replace %token NUM with typed version
    sed 's/^%token  NUM /%token  <h> NUM /' | \
    sed 's/^%token	ID EID HID HID6 AID/%token	<s> ID EID HID HID6 AID/' | \
    # Replace the stmt type reference
    sed 's/<stmt>//' | \
    cat

# Phase 3: Extract precedence declarations
echo ""
sed -n '/^%left OR/,/^%nonassoc UMINUS/p' "$INPUT"
echo ""

# Phase 4: Extract the grammar rules between %% markers and transform actions
echo "%%"
sed -n '/^%%$/,/^%%$/p' "$INPUT" | \
    tail -n +2 | head -n -1 | \
    # Replace C function calls with Go equivalents
    sed 's/gen_and(\(.*\)\.b, \(.*\)\.b)/codegen.GenAnd(\1.b, \2.b)/g' | \
    sed 's/gen_or(\(.*\)\.b, \(.*\)\.b)/codegen.GenOr(\1.b, \2.b)/g' | \
    sed 's/gen_not(\(.*\)\.b)/codegen.GenNot(\1.b)/g' | \
    # Replace $<blk>0 with cs.PeekQual() pattern
    sed 's/\$<blk>0\.q/cs.PeekQual()/g' | \
    sed 's/\$<blk>0/cs.PeekQual() \/\* TODO: was \$<blk>0 \*\//g' | \
    # Replace CHECK_PTR_VAL pattern
    sed 's/CHECK_PTR_VAL(\(.*\));/\1; if \1 == nil { return 1 } \/\* TODO: review \*\//g' | \
    # Replace CHECK_INT_VAL pattern
    sed 's/CHECK_INT_VAL(\(.*\));/\/\* TODO: CHECK_INT_VAL(\1) \*\//g' | \
    # Replace QSET macro
    sed 's/QSET(\$\$\.q, \(.*\), \(.*\), \(.*\))/$$.q = codegen.Qual{Proto: uint8(\1), Dir: uint8(\2), Addr: uint8(\3)}/g' | \
    # Replace Q_ constants
    sed 's/Q_HOST/codegen.QHost/g' | \
    sed 's/Q_NET/codegen.QNet/g' | \
    sed 's/Q_PORT\b/codegen.QPort/g' | \
    sed 's/Q_PORTRANGE/codegen.QPortrange/g' | \
    sed 's/Q_GATEWAY/codegen.QGateway/g' | \
    sed 's/Q_PROTO\b/codegen.QProto/g' | \
    sed 's/Q_PROTOCHAIN/codegen.QProtochain/g' | \
    sed 's/Q_DEFAULT/codegen.QDefault/g' | \
    sed 's/Q_UNDEF/codegen.QUndef/g' | \
    sed 's/Q_SRC/codegen.QSrc/g' | \
    sed 's/Q_DST/codegen.QDst/g' | \
    sed 's/Q_OR/codegen.QOr/g' | \
    sed 's/Q_AND/codegen.QAnd/g' | \
    sed 's/Q_ADDR1/codegen.QAddr1/g' | \
    sed 's/Q_ADDR2/codegen.QAddr2/g' | \
    sed 's/Q_ADDR3/codegen.QAddr3/g' | \
    sed 's/Q_ADDR4/codegen.QAddr4/g' | \
    sed 's/Q_RA/codegen.QRA/g' | \
    sed 's/Q_TA/codegen.QTA/g' | \
    sed 's/Q_LINK/codegen.QLink/g' | \
    sed 's/Q_IP\b/codegen.QIP/g' | \
    sed 's/Q_ARP/codegen.QARP/g' | \
    sed 's/Q_RARP/codegen.QRARP/g' | \
    sed 's/Q_SCTP/codegen.QSCTP/g' | \
    sed 's/Q_TCP/codegen.QTCP/g' | \
    sed 's/Q_UDP/codegen.QUDP/g' | \
    sed 's/Q_ICMP\b/codegen.QICMP/g' | \
    sed 's/Q_IGMP/codegen.QIGMP/g' | \
    sed 's/Q_IGRP/codegen.QIGRP/g' | \
    sed 's/Q_PIM/codegen.QPIM/g' | \
    sed 's/Q_VRRP/codegen.QVRRP/g' | \
    sed 's/Q_CARP/codegen.QCARP/g' | \
    sed 's/Q_ATALK/codegen.QAtalk/g' | \
    sed 's/Q_AARP/codegen.QAARP/g' | \
    sed 's/Q_DECNET/codegen.QDecnet/g' | \
    sed 's/Q_LAT/codegen.QLat/g' | \
    sed 's/Q_SCA/codegen.QSCA/g' | \
    sed 's/Q_MOPDL/codegen.QMopdl/g' | \
    sed 's/Q_MOPRC/codegen.QMoprc/g' | \
    sed 's/Q_IPV6/codegen.QIPv6/g' | \
    sed 's/Q_ICMPV6/codegen.QICMPv6/g' | \
    sed 's/Q_AH/codegen.QAH/g' | \
    sed 's/Q_ESP/codegen.QESP/g' | \
    sed 's/Q_ISO/codegen.QISO/g' | \
    sed 's/Q_ESIS/codegen.QESIS/g' | \
    sed 's/Q_ISIS_L1/codegen.QISISL1/g' | \
    sed 's/Q_ISIS_L2/codegen.QISISL2/g' | \
    sed 's/Q_ISIS_IIH/codegen.QISISIIH/g' | \
    sed 's/Q_ISIS_LSP/codegen.QISISLSP/g' | \
    sed 's/Q_ISIS_SNP\b/codegen.QISISSNP/g' | \
    sed 's/Q_ISIS_PSNP/codegen.QISISPSNP/g' | \
    sed 's/Q_ISIS_CSNP/codegen.QISISCSNP/g' | \
    sed 's/Q_ISIS\b/codegen.QISIS/g' | \
    sed 's/Q_CLNP/codegen.QCLNP/g' | \
    sed 's/Q_STP/codegen.QSTP/g' | \
    sed 's/Q_IPX/codegen.QIPX/g' | \
    sed 's/Q_NETBEUI/codegen.QNetbeui/g' | \
    sed 's/Q_RADIO/codegen.QRadio/g' | \
    # Replace BPF constants
    sed 's/BPF_JGT/int(bpf.BPF_JGT)/g' | \
    sed 's/BPF_JGE/int(bpf.BPF_JGE)/g' | \
    sed 's/BPF_JEQ/int(bpf.BPF_JEQ)/g' | \
    sed 's/BPF_ADD/int(bpf.BPF_ADD)/g' | \
    sed 's/BPF_SUB/int(bpf.BPF_SUB)/g' | \
    sed 's/BPF_MUL/int(bpf.BPF_MUL)/g' | \
    sed 's/BPF_DIV/int(bpf.BPF_DIV)/g' | \
    sed 's/BPF_MOD/int(bpf.BPF_MOD)/g' | \
    sed 's/BPF_AND/int(bpf.BPF_AND)/g' | \
    sed 's/BPF_OR/int(bpf.BPF_OR)/g' | \
    sed 's/BPF_XOR/int(bpf.BPF_XOR)/g' | \
    sed 's/BPF_LSH/int(bpf.BPF_LSH)/g' | \
    sed 's/BPF_RSH/int(bpf.BPF_RSH)/g' | \
    # Replace ATM constants
    sed 's/A_LANE/codegen.ALane/g' | \
    sed 's/A_METAC/codegen.AMetac/g' | \
    sed 's/A_BCC/codegen.ABCC/g' | \
    sed 's/A_OAMF4EC/codegen.AOAMF4EC/g' | \
    sed 's/A_OAMF4SC/codegen.AOAMF4SC/g' | \
    sed 's/A_SC\b/codegen.ASC/g' | \
    sed 's/A_ILMIC/codegen.AILMIC/g' | \
    sed 's/A_OAM\b/codegen.AOAM/g' | \
    sed 's/A_OAMF4\b/codegen.AOAMF4/g' | \
    sed 's/A_CONNECTMSG/codegen.AConnectmsg/g' | \
    sed 's/A_METACONNECT/codegen.AMetaconnect/g' | \
    sed 's/A_VPI/codegen.AVPI/g' | \
    sed 's/A_VCI/codegen.AVCI/g' | \
    # Replace MTP constants
    sed 's/M_FISU\b/codegen.MFISU/g' | \
    sed 's/M_LSSU\b/codegen.MLSSU/g' | \
    sed 's/M_MSU\b/codegen.MMSU/g' | \
    sed 's/MH_FISU/codegen.MHFISU/g' | \
    sed 's/MH_LSSU/codegen.MHLSSU/g' | \
    sed 's/MH_MSU/codegen.MHMSU/g' | \
    sed 's/M_SIO\b/codegen.MSIO/g' | \
    sed 's/M_OPC\b/codegen.MOPC/g' | \
    sed 's/M_DPC\b/codegen.MDPC/g' | \
    sed 's/M_SLS\b/codegen.MSLS/g' | \
    sed 's/MH_SIO/codegen.MHSIO/g' | \
    sed 's/MH_OPC/codegen.MHOPC/g' | \
    sed 's/MH_DPC/codegen.MHDPC/g' | \
    sed 's/MH_SLS/codegen.MHSLS/g' | \
    # Replace gen_* function calls with codegen.Gen* equivalents
    sed 's/gen_proto_abbrev(cstate, /codegen.GenProtoAbbrev(cs, /g' | \
    sed 's/gen_relation(cstate, /codegen.GenRelation(cs, /g' | \
    sed 's/gen_less(cstate, /codegen.GenLess(cs, int(/g' | \
    sed 's/gen_greater(cstate, /codegen.GenGreater(cs, int(/g' | \
    sed 's/gen_byteop(cstate, /codegen.GenByteop(cs, /g' | \
    sed 's/gen_broadcast(cstate, /codegen.GenBroadcast(cs, /g' | \
    sed 's/gen_multicast(cstate, /codegen.GenMulticast(cs, /g' | \
    sed 's/gen_inbound(cstate, /codegen.GenInbound(cs, /g' | \
    sed 's/gen_ifindex(cstate, /codegen.GenIfindex(cs, int(/g' | \
    sed 's/gen_vlan(cstate, /codegen.GenVlan(cs, /g' | \
    sed 's/gen_mpls(cstate, /codegen.GenMpls(cs, /g' | \
    sed 's/gen_pppoed(cstate)/codegen.GenPppoed(cs)/g' | \
    sed 's/gen_pppoes(cstate, /codegen.GenPppoes(cs, /g' | \
    sed 's/gen_geneve(cstate, /codegen.GenGeneve(cs, /g' | \
    sed 's/gen_loadi(cstate, /codegen.GenLoadi(cs, /g' | \
    sed 's/gen_load(cstate, /codegen.GenLoad(cs, /g' | \
    sed 's/gen_loadlen(cstate)/codegen.GenLoadlen(cs)/g' | \
    sed 's/gen_neg(cstate, /codegen.GenNeg(cs, /g' | \
    sed 's/gen_arth(cstate, /codegen.GenArth(cs, /g' | \
    sed 's/gen_ncode(cstate, /codegen.GenNcode(cs, /g' | \
    sed 's/gen_scode(cstate, /codegen.GenScode(cs, /g' | \
    sed 's/gen_ecode(cstate, /codegen.GenEcode(cs, /g' | \
    sed 's/gen_acode(cstate, /codegen.GenAcode(cs, /g' | \
    sed 's/gen_mcode(cstate, /codegen.GenMcode(cs, /g' | \
    sed 's/gen_mcode6(cstate, /codegen.GenMcode6(cs, /g' | \
    sed 's/gen_llc(cstate)/codegen.GenLLC(cs)/g' | \
    sed 's/gen_llc_i(cstate)/codegen.GenLLCI(cs)/g' | \
    sed 's/gen_llc_s(cstate)/codegen.GenLLCS(cs)/g' | \
    sed 's/gen_llc_u(cstate)/codegen.GenLLCU(cs)/g' | \
    sed 's/gen_llc_s_subtype(cstate, /codegen.GenLLCSSubtype(cs, /g' | \
    sed 's/gen_llc_u_subtype(cstate, /codegen.GenLLCUSubtype(cs, /g' | \
    sed 's/gen_atmtype_abbrev(cstate, /codegen.GenAtmtypeAbbrev(cs, /g' | \
    sed 's/gen_atmmulti_abbrev(cstate, /codegen.GenAtmmultiAbbrev(cs, /g' | \
    sed 's/gen_atmfield_code(cstate, /codegen.GenAtmfieldCode(cs, /g' | \
    sed 's/gen_mtp2type_abbrev(cstate, /codegen.GenMtp2typeAbbrev(cs, /g' | \
    sed 's/gen_mtp3field_code(cstate, /codegen.GenMtp3fieldCode(cs, /g' | \
    sed 's/gen_pf_ifname(cstate, /codegen.GenPfIfname(cs, /g' | \
    sed 's/gen_pf_ruleset(cstate, /codegen.GenPfRuleset(cs, /g' | \
    sed 's/gen_pf_rnr(cstate, /codegen.GenPfRnr(cs, int(/g' | \
    sed 's/gen_pf_srnr(cstate, /codegen.GenPfSrnr(cs, int(/g' | \
    sed 's/gen_pf_reason(cstate, /codegen.GenPfReason(cs, /g' | \
    sed 's/gen_pf_action(cstate, /codegen.GenPfAction(cs, /g' | \
    sed 's/gen_p80211_type(cstate, /codegen.GenP80211Type(cs, /g' | \
    sed 's/gen_p80211_fcdir(cstate, /codegen.GenP80211Fcdir(cs, /g' | \
    sed 's/finish_parse(cstate, /codegen.FinishParse(cs, /g' | \
    # Replace cstate access pattern
    sed 's/bpf_set_error(cstate, /cs.SetError(fmt.Errorf(/g' | \
    # Replace NULL with nil
    sed 's/NULL/nil/g' | \
    # Remove C casts
    sed 's/(int)//g' | \
    sed 's/(u_int)//g' | \
    # Replace C-style comments
    sed 's|/\* null \*/|/* empty */|g' | \
    # Add /* TODO */ markers for sections needing manual work
    sed 's/YYABORT/return 1 \/\* TODO: was YYABORT \*\//g' | \
    sed 's/#ifdef.*/#if 0 \/\* TODO: conditional compilation \*\//g' | \
    sed 's/#else/\/\* TODO: else branch \*\//g' | \
    sed 's/#endif.*/\/\* TODO: endif \*\//g' | \
    cat

echo "%%"

echo ""
echo "// NOTE: This is a DRAFT output. You must manually:"
echo "// 1. Add 'cs := yylex.(*parserLex).cs' at the start of each action that uses cs"
echo "// 2. Fix push/pop qualifier stack in head/rterm productions"
echo "// 3. Review all /* TODO */ markers"
echo "// 4. Replace remaining C constructs (casts, pointer checks, etc.)"
echo "// 5. Fix lookup table references (str2tok, ieee80211_types, etc.)"
echo "// 6. Run: goyacc -o grammar.go -p yy grammar.y"
echo "// 7. Verify 38 shift/reduce conflicts (matching libpcap)"
