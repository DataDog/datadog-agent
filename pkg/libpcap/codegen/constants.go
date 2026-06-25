// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package codegen implements BPF code generation from parsed filter expressions.
// It is a port of libpcap's gencode.c and gencode.h.
package codegen

// Address qualifiers
const (
	QHost       = 1
	QNet        = 2
	QPort       = 3
	QGateway    = 4
	QProto      = 5
	QProtochain = 6
	QPortrange  = 7
)

// Protocol qualifiers
const (
	QLink     = 1
	QIP       = 2
	QARP      = 3
	QRARP     = 4
	QSCTP     = 5
	QTCP      = 6
	QUDP      = 7
	QICMP     = 8
	QIGMP     = 9
	QIGRP     = 10
	QAtalk    = 11
	QDecnet   = 12
	QLat      = 13
	QSCA      = 14
	QMoprc    = 15
	QMopdl    = 16
	QIPv6     = 17
	QICMPv6   = 18
	QAH       = 19
	QESP      = 20
	QPIM      = 21
	QVRRP     = 22
	QAARP     = 23
	QISO      = 24
	QESIS     = 25
	QISIS     = 26
	QCLNP     = 27
	QSTP      = 28
	QIPX      = 29
	QNetbeui  = 30
	QISISL1   = 31
	QISISL2   = 32
	QISISIIH  = 33
	QISISSNP  = 34
	QISISCSNP = 35
	QISISPSNP = 36
	QISISLSP  = 37
	QRadio    = 38
	QCARP     = 39
)

// Directional qualifiers
const (
	QDefault = 0
	QSrc     = 1
	QDst     = 2
	QOr      = 3
	QAnd     = 4
	QAddr1   = 5
	QAddr2   = 6
	QAddr3   = 7
	QAddr4   = 8
	QRA      = 9
	QTA      = 10
	QUndef   = 255
)

// ATM types
const (
	AMetac       = 22
	ABCC         = 23
	AOAMF4SC     = 24
	AOAMF4EC     = 25
	ASC          = 26
	AILMIC       = 27
	AOAM         = 28
	AOAMF4       = 29
	ALane        = 30
	ALLC         = 31
	ASetup       = 41
	ACallproceed = 42
	AConnect     = 43
	AConnectack  = 44
	ARelease     = 45
	AReleaseDone = 46
	AVPI         = 51
	AVCI         = 52
	APrototype   = 53
	AMsgtype     = 54
	ACallreftype = 55
	AConnectmsg  = 70
	AMetaconnect = 71
)

// MTP2 types
const (
	MFISU  = 22
	MLSSU  = 23
	MMSU   = 24
	MHFISU = 25
	MHLSSU = 26
	MHMSU  = 27
)

// MTP3 field types
const (
	MSIO  = 1
	MOPC  = 2
	MDPC  = 3
	MSLS  = 4
	MHSIO = 5
	MHOPC = 6
	MHDPC = 7
	MHSLS = 8
)

// ProtoUndef indicates an undefined protocol
const ProtoUndef = -1

// Common Ethertypes
const (
	EthertypeIP        = 0x0800
	EthertypeARP       = 0x0806
	EthertypeRevarp    = 0x8035
	EthertypeIPv6      = 0x86dd
	EthertypeAtalk     = 0x809b
	EthertypeDN        = 0x6003
	EthertypeAARP      = 0x80f3
	EthertypeIPX       = 0x8137
	EthertypeVLAN      = 0x8100
	EthertypeQinQ      = 0x88a8
	EthertypePPPoED    = 0x8863
	EthertypePPPoES    = 0x8864
	EthertypeMPLS      = 0x8847
	EthertypeMPLSMulti = 0x8848
	EthertypeLAT       = 0x6004
	EthertypeSCA       = 0x6007
	EthertypeMoprc     = 0x6002
	EthertypeMopdl     = 0x6001
)

// LLC SAP values
const (
	LLCSAPNull    = 0x00
	LLCSAPGlobal  = 0xff
	LLCSAP8021D   = 0x42 // STP
	LLCSAPIP      = 0x06
	LLCSAPISONs   = 0xfe
	LLCSAPIPX     = 0xe0
	LLCSAPNetbeui = 0xf0
	LLCSAPSNAP    = 0xaa
)

// IP protocol numbers
const (
	IPProtoTCP    = 6
	IPProtoUDP    = 17
	IPProtoSCTP   = 132
	IPProtoICMP   = 1
	IPProtoIGMP   = 2
	IPProtoIGRP   = 9
	IPProtoGRE    = 47
	IPProtoESP    = 50
	IPProtoAH     = 51
	IPProtoICMPv6 = 58
	IPProtoPIM    = 103
	IPProtoVRRP   = 112
	IPProtoCARP   = 112 // same as VRRP
)

// Ethernet header size
const EtherHdrLen = 14

// EtherMTU is the maximum length field value for 802.3 frames
const EtherMTU = 1500

// NAtoms is the total number of atomic entities (BPF_MEMWORDS + A + X).
const NAtoms = 18 // BPF_MEMWORDS(16) + 2
