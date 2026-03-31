%{
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Port of libpcap's grammar.y.in to goyacc.

package grammar

import (
	"github.com/DataDog/datadog-agent/pkg/libpcap/codegen"
	"github.com/DataDog/datadog-agent/pkg/libpcap/bpf"
)

// Ensure imports are used.
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

%type	<blk>	expr id nid pid term rterm qid
%type	<blk>	head
%type	<i>	pqual dqual aqual ndaqual
%type	<a>	arth narth
%type	<i>	byteop pname relop irelop
%type	<h>	pnum
%type	<blk>	and or paren not null prog
%type	<rblk>	other pfvar p80211 pllc
%type	<i>	atmtype atmmultitype
%type	<blk>	atmfield
%type	<blk>	atmfieldvalue atmvalue atmlistvalue
%type	<i>	mtp2type
%type	<blk>	mtp3field
%type	<blk>	mtp3fieldvalue mtp3value mtp3listvalue

%type	<i>	action reason type subtype type_subtype dir

%token  DST SRC HOST GATEWAY
%token  NET NETMASK PORT PORTRANGE LESS GREATER PROTO PROTOCHAIN CBYTE
%token  ARP RARP IP SCTP TCP UDP ICMP IGMP IGRP PIM VRRP CARP
%token  ATALK AARP DECNET LAT SCA MOPRC MOPDL
%token  TK_BROADCAST TK_MULTICAST
%token  <h> NUM
%token  INBOUND OUTBOUND
%token  IFINDEX
%token  PF_IFNAME PF_RSET PF_RNR PF_SRNR PF_REASON PF_ACTION
%token	TYPE SUBTYPE DIR ADDR1 ADDR2 ADDR3 ADDR4 RA TA
%token  LINK
%token	GEQ LEQ NEQ
%token	<s> ID EID HID HID6 AID
%token	LSH RSH
%token  LEN
%token  IPV6 ICMPV6 AH ESP
%token	VLAN MPLS
%token	PPPOED PPPOES GENEVE
%token  ISO ESIS CLNP ISIS L1 L2 IIH LSP SNP CSNP PSNP
%token  STP
%token  IPX
%token  NETBEUI
%token	LANE LLC METAC BCC SC ILMIC OAMF4EC OAMF4SC
%token	OAM OAMF4 CONNECTMSG METACONNECT
%token	VPI VCI
%token	RADIO
%token	FISU LSSU MSU HFISU HLSSU HMSU
%token	SIO OPC DPC SLS HSIO HOPC HDPC HSLS
%token	LEX_ERROR
%token	AND OR

%left OR AND
%nonassoc  '!'
%left '|'
%left '&'
%left LSH RSH
%left '+' '-'
%left '*' '/'
%nonassoc UMINUS

%%
prog:	  null expr
	{
		cs := yylex.(*parserLex).cs
		if err := codegen.FinishParse(cs, $2.b); err != nil {
			yylex.Error(err.Error())
			return 1
		}
	}
	| null
	;
null:	  /* empty */
	{
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	;
expr:	  term
	| expr and term		{ codegen.GenAnd($1.b, $3.b); $$ = $3 }
	| expr and id		{ codegen.GenAnd($1.b, $3.b); $$ = $3 }
	| expr or term		{ codegen.GenOr($1.b, $3.b); $$ = $3 }
	| expr or id		{ codegen.GenOr($1.b, $3.b); $$ = $3 }
	;
and:	  AND
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
	}
	;
or:	  OR
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
	}
	;
id:	  nid
	| pnum
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
		$$.b = codegen.GenNcode(cs, "", $1, $$.q)
		if $$.b == nil { return 1 }
	}
	| paren pid ')'		{ $$ = $2 }
	;
nid:	  ID
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
		if $1 == "" { return 1 }
		$$.b = codegen.GenScode(cs, $1, $$.q)
		if $$.b == nil { return 1 }
	}
	| HID '/' NUM
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenMcode(cs, $1, "", $3, $$.q)
		if $$.b == nil { return 1 }
	}
	| HID NETMASK HID
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenMcode(cs, $1, $3, 0, $$.q)
		if $$.b == nil { return 1 }
	}
	| HID
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenNcode(cs, $1, 0, $$.q)
		if $$.b == nil { return 1 }
	}
	| HID6 '/' NUM
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenMcode6(cs, $1, $3, $$.q)
		if $$.b == nil { return 1 }
	}
	| HID6
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenMcode6(cs, $1, 128, $$.q)
		if $$.b == nil { return 1 }
	}
	| EID
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenEcode(cs, $1, $$.q)
		if $$.b == nil { return 1 }
	}
	| AID
	{
		cs := yylex.(*parserLex).cs
		if $1 == "" { return 1 }
		$$.q = cs.PeekQual()
		$$.b = codegen.GenAcode(cs, $1, $$.q)
		if $$.b == nil { return 1 }
	}
	| not id		{ codegen.GenNot($2.b); $$ = $2 }
	;
not:	  '!'
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
	}
	;
paren:	  '('
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
	}
	;
pid:	  nid
	| qid and id		{ codegen.GenAnd($1.b, $3.b); $$ = $3 }
	| qid or id		{ codegen.GenOr($1.b, $3.b); $$ = $3 }
	;
qid:	  pnum
	{
		cs := yylex.(*parserLex).cs
		$$.q = cs.PeekQual()
		$$.b = codegen.GenNcode(cs, "", $1, $$.q)
		if $$.b == nil { return 1 }
	}
	| pid
	;
term:	  rterm
	| not term		{ codegen.GenNot($2.b); $$ = $2 }
	;
head:	  pqual dqual aqual
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: uint8($2), Addr: uint8($3)}
		cs.PushQual($$.q)
	}
	| pqual dqual
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: uint8($2), Addr: codegen.QDefault}
		cs.PushQual($$.q)
	}
	| pqual aqual
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: codegen.QDefault, Addr: uint8($2)}
		cs.PushQual($$.q)
	}
	| pqual PROTO
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: codegen.QDefault, Addr: codegen.QProto}
		cs.PushQual($$.q)
	}
	| pqual PROTOCHAIN
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: codegen.QDefault, Addr: codegen.QProtochain}
		cs.PushQual($$.q)
	}
	| pqual ndaqual
	{
		cs := yylex.(*parserLex).cs
		$$.q = codegen.Qual{Proto: uint8($1), Dir: codegen.QDefault, Addr: uint8($2)}
		cs.PushQual($$.q)
	}
	;
rterm:	  head id
	{
		cs := yylex.(*parserLex).cs
		cs.PopQual()
		$$ = $2
	}
	| paren expr ')'	{ $$.b = $2.b; $$.q = $1.q }
	| pname
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenProtoAbbrev(cs, $1)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| arth relop arth
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenRelation(cs, $2, $1, $3, 0)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| arth irelop arth
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenRelation(cs, $2, $1, $3, 1)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| other
	{
		$$.b = $1
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| atmtype
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenAtmtypeAbbrev(cs, $1)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| atmmultitype
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenAtmmultiAbbrev(cs, $1)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| atmfield atmvalue	{ $$.b = $2.b; $$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef} }
	| mtp2type
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenMtp2typeAbbrev(cs, $1)
		if $$.b == nil { return 1 }
		$$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef}
	}
	| mtp3field mtp3value	{ $$.b = $2.b; $$.q = codegen.Qual{Addr: codegen.QUndef, Proto: codegen.QUndef, Dir: codegen.QUndef} }
	;
pqual:	  pname
	|		{ $$ = codegen.QDefault }
	;
dqual:	  SRC			{ $$ = codegen.QSrc }
	| DST			{ $$ = codegen.QDst }
	| SRC OR DST		{ $$ = codegen.QOr }
	| DST OR SRC		{ $$ = codegen.QOr }
	| SRC AND DST		{ $$ = codegen.QAnd }
	| DST AND SRC		{ $$ = codegen.QAnd }
	| ADDR1			{ $$ = codegen.QAddr1 }
	| ADDR2			{ $$ = codegen.QAddr2 }
	| ADDR3			{ $$ = codegen.QAddr3 }
	| ADDR4			{ $$ = codegen.QAddr4 }
	| RA			{ $$ = codegen.QRA }
	| TA			{ $$ = codegen.QTA }
	;
aqual:	  HOST			{ $$ = codegen.QHost }
	| NET			{ $$ = codegen.QNet }
	| PORT			{ $$ = codegen.QPort }
	| PORTRANGE		{ $$ = codegen.QPortrange }
	;
ndaqual:  GATEWAY		{ $$ = codegen.QGateway }
	;
pname:	  LINK			{ $$ = codegen.QLink }
	| IP			{ $$ = codegen.QIP }
	| ARP			{ $$ = codegen.QARP }
	| RARP			{ $$ = codegen.QRARP }
	| SCTP			{ $$ = codegen.QSCTP }
	| TCP			{ $$ = codegen.QTCP }
	| UDP			{ $$ = codegen.QUDP }
	| ICMP			{ $$ = codegen.QICMP }
	| IGMP			{ $$ = codegen.QIGMP }
	| IGRP			{ $$ = codegen.QIGRP }
	| PIM			{ $$ = codegen.QPIM }
	| VRRP			{ $$ = codegen.QVRRP }
	| CARP			{ $$ = codegen.QCARP }
	| ATALK			{ $$ = codegen.QAtalk }
	| AARP			{ $$ = codegen.QAARP }
	| DECNET		{ $$ = codegen.QDecnet }
	| LAT			{ $$ = codegen.QLat }
	| SCA			{ $$ = codegen.QSCA }
	| MOPDL			{ $$ = codegen.QMopdl }
	| MOPRC			{ $$ = codegen.QMoprc }
	| IPV6			{ $$ = codegen.QIPv6 }
	| ICMPV6		{ $$ = codegen.QICMPv6 }
	| AH			{ $$ = codegen.QAH }
	| ESP			{ $$ = codegen.QESP }
	| ISO			{ $$ = codegen.QISO }
	| ESIS			{ $$ = codegen.QESIS }
	| ISIS			{ $$ = codegen.QISIS }
	| L1			{ $$ = codegen.QISISL1 }
	| L2			{ $$ = codegen.QISISL2 }
	| IIH			{ $$ = codegen.QISISIIH }
	| LSP			{ $$ = codegen.QISISLSP }
	| SNP			{ $$ = codegen.QISISSNP }
	| PSNP			{ $$ = codegen.QISISPSNP }
	| CSNP			{ $$ = codegen.QISISCSNP }
	| CLNP			{ $$ = codegen.QCLNP }
	| STP			{ $$ = codegen.QSTP }
	| IPX			{ $$ = codegen.QIPX }
	| NETBEUI		{ $$ = codegen.QNetbeui }
	| RADIO			{ $$ = codegen.QRadio }
	;
other:	  pqual TK_BROADCAST
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenBroadcast(cs, $1)
		if $$ == nil { return 1 }
	}
	| pqual TK_MULTICAST
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenMulticast(cs, $1)
		if $$ == nil { return 1 }
	}
	| LESS NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLess(cs, int($2))
		if $$ == nil { return 1 }
	}
	| GREATER NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenGreater(cs, int($2))
		if $$ == nil { return 1 }
	}
	| CBYTE NUM byteop NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenByteop(cs, $3, int($2), $4)
		if $$ == nil { return 1 }
	}
	| INBOUND
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenInbound(cs, 0)
		if $$ == nil { return 1 }
	}
	| OUTBOUND
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenInbound(cs, 1)
		if $$ == nil { return 1 }
	}
	| IFINDEX NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenIfindex(cs, int($2))
		if $$ == nil { return 1 }
	}
	| VLAN pnum
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenVlan(cs, $2, 1)
		if $$ == nil { return 1 }
	}
	| VLAN
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenVlan(cs, 0, 0)
		if $$ == nil { return 1 }
	}
	| MPLS pnum
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenMpls(cs, $2, 1)
		if $$ == nil { return 1 }
	}
	| MPLS
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenMpls(cs, 0, 0)
		if $$ == nil { return 1 }
	}
	| PPPOED
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPppoed(cs)
		if $$ == nil { return 1 }
	}
	| PPPOES pnum
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPppoes(cs, $2, 1)
		if $$ == nil { return 1 }
	}
	| PPPOES
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPppoes(cs, 0, 0)
		if $$ == nil { return 1 }
	}
	| GENEVE pnum
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenGeneve(cs, $2, 1)
		if $$ == nil { return 1 }
	}
	| GENEVE
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenGeneve(cs, 0, 0)
		if $$ == nil { return 1 }
	}
	| pfvar			{ $$ = $1 }
	| pqual p80211		{ $$ = $2 }
	| pllc			{ $$ = $1 }
	;
pfvar:	  PF_IFNAME ID
	{
		cs := yylex.(*parserLex).cs
		if $2 == "" { return 1 }
		$$ = codegen.GenPfIfname(cs, $2)
		if $$ == nil { return 1 }
	}
	| PF_RSET ID
	{
		cs := yylex.(*parserLex).cs
		if $2 == "" { return 1 }
		$$ = codegen.GenPfRuleset(cs, $2)
		if $$ == nil { return 1 }
	}
	| PF_RNR NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPfRnr(cs, int($2))
		if $$ == nil { return 1 }
	}
	| PF_SRNR NUM
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPfSrnr(cs, int($2))
		if $$ == nil { return 1 }
	}
	| PF_REASON reason
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPfReason(cs, $2)
		if $$ == nil { return 1 }
	}
	| PF_ACTION action
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenPfAction(cs, $2)
		if $$ == nil { return 1 }
	}
	;
p80211:   TYPE type SUBTYPE subtype
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenP80211Type(cs, uint32($2 | $4), 0x0c|0xf0)
		if $$ == nil { return 1 }
	}
	| TYPE type
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenP80211Type(cs, uint32($2), 0x0c)
		if $$ == nil { return 1 }
	}
	| SUBTYPE type_subtype
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenP80211Type(cs, uint32($2), 0x0c|0xf0)
		if $$ == nil { return 1 }
	}
	| DIR dir
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenP80211Fcdir(cs, uint32($2))
		if $$ == nil { return 1 }
	}
	;
type:	  NUM			{ $$ = int($1) }
	| ID			{ $$ = 0 /* simplified: str2tok lookup */ }
	;
subtype:  NUM			{ $$ = int($1) }
	| ID			{ $$ = 0 /* simplified */ }
	;
type_subtype: ID		{ $$ = 0 /* simplified */ }
	;
pllc:	  LLC
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLLC(cs)
		if $$ == nil { return 1 }
	}
	| LLC ID
	{
		cs := yylex.(*parserLex).cs
		if $2 == "" { return 1 }
		switch $2 {
		case "i":
			$$ = codegen.GenLLCI(cs)
		case "s":
			$$ = codegen.GenLLCS(cs)
		case "u":
			$$ = codegen.GenLLCU(cs)
		default:
			$$ = codegen.GenLLC(cs)
		}
		if $$ == nil { return 1 }
	}
	| LLC PF_RNR
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLLCSSubtype(cs, 0x05) /* LLC_RNR */
		if $$ == nil { return 1 }
	}
	;
dir:	  NUM			{ $$ = int($1) }
	| ID			{ $$ = 0 /* simplified: nods/tods/fromds/dstods lookup */ }
	;
reason:	  NUM			{ $$ = int($1) }
	| ID			{ $$ = 0 /* simplified: pfreason_to_num lookup */ }
	;
action:	  ID			{ $$ = 0 /* simplified: pfaction_to_num lookup */ }
	;
relop:	  '>'			{ $$ = int(bpf.BPF_JGT) }
	| GEQ			{ $$ = int(bpf.BPF_JGE) }
	| '='			{ $$ = int(bpf.BPF_JEQ) }
	;
irelop:	  LEQ			{ $$ = int(bpf.BPF_JGT) }
	| '<'			{ $$ = int(bpf.BPF_JGE) }
	| NEQ			{ $$ = int(bpf.BPF_JEQ) }
	;
arth:	  pnum
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLoadi(cs, $1)
		if $$ == nil { return 1 }
	}
	| narth
	;
narth:	  pname '[' arth ']'
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLoad(cs, $1, $3, 1)
		if $$ == nil { return 1 }
	}
	| pname '[' arth ':' NUM ']'
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLoad(cs, $1, $3, $5)
		if $$ == nil { return 1 }
	}
	| arth '+' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_ADD), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '-' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_SUB), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '*' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_MUL), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '/' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_DIV), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '%' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_MOD), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '&' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_AND), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '|' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_OR), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth '^' arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_XOR), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth LSH arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_LSH), $1, $3)
		if $$ == nil { return 1 }
	}
	| arth RSH arth
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenArth(cs, int(bpf.BPF_RSH), $1, $3)
		if $$ == nil { return 1 }
	}
	| '-' arth %prec UMINUS
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenNeg(cs, $2)
		if $$ == nil { return 1 }
	}
	| paren narth ')'	{ $$ = $2 }
	| LEN
	{
		cs := yylex.(*parserLex).cs
		$$ = codegen.GenLoadlen(cs)
		if $$ == nil { return 1 }
	}
	;
byteop:	  '&'			{ $$ = '&' }
	| '|'			{ $$ = '|' }
	| '<'			{ $$ = '<' }
	| '>'			{ $$ = '>' }
	| '='			{ $$ = '=' }
	;
pnum:	  NUM
	| paren pnum ')'	{ $$ = $2 }
	;
atmtype:  LANE			{ $$ = codegen.ALane }
	| METAC			{ $$ = codegen.AMetac }
	| BCC			{ $$ = codegen.ABCC }
	| OAMF4EC		{ $$ = codegen.AOAMF4EC }
	| OAMF4SC		{ $$ = codegen.AOAMF4SC }
	| SC			{ $$ = codegen.ASC }
	| ILMIC			{ $$ = codegen.AILMIC }
	;
atmmultitype: OAM		{ $$ = codegen.AOAM }
	| OAMF4			{ $$ = codegen.AOAMF4 }
	| CONNECTMSG		{ $$ = codegen.AConnectmsg }
	| METACONNECT		{ $$ = codegen.AMetaconnect }
	;
atmfield: VPI			{ $$.atmfieldtype = codegen.AVPI }
	| VCI			{ $$.atmfieldtype = codegen.AVCI }
	;
atmvalue: atmfieldvalue
	| relop NUM
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenAtmfieldCode(cs, $<blk>0.atmfieldtype, $2, $1, 0)
		if $$.b == nil { return 1 }
	}
	| irelop NUM
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenAtmfieldCode(cs, $<blk>0.atmfieldtype, $2, $1, 1)
		if $$.b == nil { return 1 }
	}
	| paren atmlistvalue ')'	{ $$.b = $2.b }
	;
atmfieldvalue: NUM
	{
		cs := yylex.(*parserLex).cs
		$$.atmfieldtype = $<blk>0.atmfieldtype
		$$.b = codegen.GenAtmfieldCode(cs, $$.atmfieldtype, $1, int(bpf.BPF_JEQ), 0)
		if $$.b == nil { return 1 }
	}
	;
atmlistvalue: atmfieldvalue
	| atmlistvalue or atmfieldvalue { codegen.GenOr($1.b, $3.b); $$ = $3 }
	;
mtp2type: FISU			{ $$ = codegen.MFISU }
	| LSSU			{ $$ = codegen.MLSSU }
	| MSU			{ $$ = codegen.MMSU }
	| HFISU			{ $$ = codegen.MHFISU }
	| HLSSU			{ $$ = codegen.MHLSSU }
	| HMSU			{ $$ = codegen.MHMSU }
	;
mtp3field: SIO			{ $$.mtp3fieldtype = codegen.MSIO }
	| OPC			{ $$.mtp3fieldtype = codegen.MOPC }
	| DPC			{ $$.mtp3fieldtype = codegen.MDPC }
	| SLS			{ $$.mtp3fieldtype = codegen.MSLS }
	| HSIO			{ $$.mtp3fieldtype = codegen.MHSIO }
	| HOPC			{ $$.mtp3fieldtype = codegen.MHOPC }
	| HDPC			{ $$.mtp3fieldtype = codegen.MHDPC }
	| HSLS			{ $$.mtp3fieldtype = codegen.MHSLS }
	;
mtp3value: mtp3fieldvalue
	| relop NUM
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenMtp3fieldCode(cs, $<blk>0.mtp3fieldtype, $2, $1, 0)
		if $$.b == nil { return 1 }
	}
	| irelop NUM
	{
		cs := yylex.(*parserLex).cs
		$$.b = codegen.GenMtp3fieldCode(cs, $<blk>0.mtp3fieldtype, $2, $1, 1)
		if $$.b == nil { return 1 }
	}
	| paren mtp3listvalue ')'	{ $$.b = $2.b }
	;
mtp3fieldvalue: NUM
	{
		cs := yylex.(*parserLex).cs
		$$.mtp3fieldtype = $<blk>0.mtp3fieldtype
		$$.b = codegen.GenMtp3fieldCode(cs, $$.mtp3fieldtype, $1, int(bpf.BPF_JEQ), 0)
		if $$.b == nil { return 1 }
	}
	;
mtp3listvalue: mtp3fieldvalue
	| mtp3listvalue or mtp3fieldvalue { codegen.GenOr($1.b, $3.b); $$ = $3 }
	;
%%
