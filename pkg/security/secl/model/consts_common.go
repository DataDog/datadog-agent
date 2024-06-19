// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package model holds model related files
package model

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model/usersession"
)

const (
	// MaxSegmentLength defines the maximum length of each segment of a path
	MaxSegmentLength = 255

	// MaxPathDepth defines the maximum depth of a path
	// see pkg/security/ebpf/c/dentry_resolver.h: DR_MAX_TAIL_CALL * DR_MAX_ITERATION_DEPTH
	MaxPathDepth = 1363

	// MaxBpfObjName defines the maximum length of a Bpf object name
	MaxBpfObjName = 16

	// PathSuffix defines the suffix used for path fields
	PathSuffix = ".path"

	// NameSuffix defines the suffix used for name fields
	NameSuffix = ".name"

	// ContainerIDLen defines the length of a container ID
	ContainerIDLen = sha256.Size * 2

	// MaxSymlinks maximum symlinks captured
	MaxSymlinks = 2

	// MaxTracedCgroupsCount hard limit for the count of traced cgroups
	MaxTracedCgroupsCount = 128
)

const (
	// EventFlagsAsync async event
	EventFlagsAsync = 1 << iota

	// EventFlagsSavedByAD saved by ad
	EventFlagsSavedByAD

	// EventFlagsActivityDumpSample an AD sample
	EventFlagsActivityDumpSample

	// EventFlagsSecurityProfileInProfile true if the event was found in a profile
	EventFlagsSecurityProfileInProfile

	// EventFlagsAnomalyDetectionEvent true if the event is marked as being an anomaly
	EventFlagsAnomalyDetectionEvent

	// EventFlagsHasActiveActivityDump true if the event has an active activity dump associated to it
	EventFlagsHasActiveActivityDump
)

const (
	// IMDSRequestType is used to specify that the event is an IDMS request event
	IMDSRequestType = "request"
	// IMDSResponseType is used to specify that the event is an IMDS response event
	IMDSResponseType = "response"
	// IMDSAWSCloudProvider is used to report that the IMDS event is for AWS
	IMDSAWSCloudProvider = "aws"
	// IMDSGCPCloudProvider is used to report that the IMDS event is for GCP
	IMDSGCPCloudProvider = "gcp"
	// IMDSAzureCloudProvider is used to report that the IMDS event is for Azure
	IMDSAzureCloudProvider = "azure"
	// IMDSIBMCloudProvider is used to report that the IMDS event is for ibm
	IMDSIBMCloudProvider = "ibm"
	// IMDSOracleCloudProvider is used to report that the IMDS event is for Oracle
	IMDSOracleCloudProvider = "oracle"
)

var (
	// DNSQTypeConstants see https://www.iana.org/assignments/dns-parameters/dns-parameters.xhtml
	// generate_constants:DNS qtypes,DNS qtypes are the supported DNS query types.
	DNSQTypeConstants = map[string]int{
		"None":       0,
		"A":          1,
		"NS":         2,
		"MD":         3,
		"MF":         4,
		"CNAME":      5,
		"SOA":        6,
		"MB":         7,
		"MG":         8,
		"MR":         9,
		"NULL":       10,
		"PTR":        12,
		"HINFO":      13,
		"MINFO":      14,
		"MX":         15,
		"TXT":        16,
		"RP":         17,
		"AFSDB":      18,
		"X25":        19,
		"ISDN":       20,
		"RT":         21,
		"NSAPPTR":    23,
		"SIG":        24,
		"KEY":        25,
		"PX":         26,
		"GPOS":       27,
		"AAAA":       28,
		"LOC":        29,
		"NXT":        30,
		"EID":        31,
		"NIMLOC":     32,
		"SRV":        33,
		"ATMA":       34,
		"NAPTR":      35,
		"KX":         36,
		"CERT":       37,
		"DNAME":      39,
		"OPT":        41,
		"APL":        42,
		"DS":         43,
		"SSHFP":      44,
		"RRSIG":      46,
		"NSEC":       47,
		"DNSKEY":     48,
		"DHCID":      49,
		"NSEC3":      50,
		"NSEC3PARAM": 51,
		"TLSA":       52,
		"SMIMEA":     53,
		"HIP":        55,
		"NINFO":      56,
		"RKEY":       57,
		"TALINK":     58,
		"CDS":        59,
		"CDNSKEY":    60,
		"OPENPGPKEY": 61,
		"CSYNC":      62,
		"ZONEMD":     63,
		"SVCB":       64,
		"HTTPS":      65,
		"SPF":        99,
		"UINFO":      100,
		"UID":        101,
		"GID":        102,
		"UNSPEC":     103,
		"NID":        104,
		"L32":        105,
		"L64":        106,
		"LP":         107,
		"EUI48":      108,
		"EUI64":      109,
		"URI":        256,
		"CAA":        257,
		"AVC":        258,
		"TKEY":       249,
		"TSIG":       250,
		"IXFR":       251,
		"AXFR":       252,
		"MAILB":      253,
		"MAILA":      254,
		"ANY":        255,
		"TA":         32768,
		"DLV":        32769,
		"Reserved":   65535,
	}

	// DNSQClassConstants see https://www.iana.org/assignments/dns-parameters/dns-parameters.xhtml
	// generate_constants:DNS qclasses,DNS qclasses are the supported DNS query classes.
	DNSQClassConstants = map[string]int{
		"CLASS_INET":   1,
		"CLASS_CSNET":  2,
		"CLASS_CHAOS":  3,
		"CLASS_HESIOD": 4,
		"CLASS_NONE":   254,
		"CLASS_ANY":    255,
	}

	// BooleanConstants holds the evaluator for boolean constants
	// generate_constants:Boolean constants,Boolean constants are the supported boolean constants.
	BooleanConstants = map[string]interface{}{
		// boolean
		"true":  &eval.BoolEvaluator{Value: true},
		"false": &eval.BoolEvaluator{Value: false},
	}

	// seclConstants are constants supported in runtime security agent rules
	seclConstants = map[string]interface{}{}

	// L3ProtocolConstants is the list of supported L3 protocols
	// generate_constants:L3 protocols,L3 protocols are the supported Layer 3 protocols.
	L3ProtocolConstants = map[string]L3Protocol{
		"ETH_P_LOOP":            EthPLOOP,
		"ETH_P_PUP":             EthPPUP,
		"ETH_P_PUPAT":           EthPPUPAT,
		"ETH_P_TSN":             EthPTSN,
		"ETH_P_IP":              EthPIP,
		"ETH_P_X25":             EthPX25,
		"ETH_P_ARP":             EthPARP,
		"ETH_P_BPQ":             EthPBPQ,
		"ETH_P_IEEEPUP":         EthPIEEEPUP,
		"ETH_P_IEEEPUPAT":       EthPIEEEPUPAT,
		"ETH_P_BATMAN":          EthPBATMAN,
		"ETH_P_DEC":             EthPDEC,
		"ETH_P_DNADL":           EthPDNADL,
		"ETH_P_DNARC":           EthPDNARC,
		"ETH_P_DNART":           EthPDNART,
		"ETH_P_LAT":             EthPLAT,
		"ETH_P_DIAG":            EthPDIAG,
		"ETH_P_CUST":            EthPCUST,
		"ETH_P_SCA":             EthPSCA,
		"ETH_P_TEB":             EthPTEB,
		"ETH_P_RARP":            EthPRARP,
		"ETH_P_ATALK":           EthPATALK,
		"ETH_P_AARP":            EthPAARP,
		"ETH_P_8021_Q":          EthP8021Q,
		"ETH_P_ERSPAN":          EthPERSPAN,
		"ETH_P_IPX":             EthPIPX,
		"ETH_P_IPV6":            EthPIPV6,
		"ETH_P_PAUSE":           EthPPAUSE,
		"ETH_P_SLOW":            EthPSLOW,
		"ETH_P_WCCP":            EthPWCCP,
		"ETH_P_MPLSUC":          EthPMPLSUC,
		"ETH_P_MPLSMC":          EthPMPLSMC,
		"ETH_P_ATMMPOA":         EthPATMMPOA,
		"ETH_P_PPPDISC":         EthPPPPDISC,
		"ETH_P_PPPSES":          EthPPPPSES,
		"ETH_P__LINK_CTL":       EthPLinkCTL,
		"ETH_P_ATMFATE":         EthPATMFATE,
		"ETH_P_PAE":             EthPPAE,
		"ETH_P_AOE":             EthPAOE,
		"ETH_P_8021_AD":         EthP8021AD,
		"ETH_P_802_EX1":         EthP802EX1,
		"ETH_P_TIPC":            EthPTIPC,
		"ETH_P_MACSEC":          EthPMACSEC,
		"ETH_P_8021_AH":         EthP8021AH,
		"ETH_P_MVRP":            EthPMVRP,
		"ETH_P_1588":            EthP1588,
		"ETH_P_NCSI":            EthPNCSI,
		"ETH_P_PRP":             EthPPRP,
		"ETH_P_FCOE":            EthPFCOE,
		"ETH_P_IBOE":            EthPIBOE,
		"ETH_P_TDLS":            EthPTDLS,
		"ETH_P_FIP":             EthPFIP,
		"ETH_P_80221":           EthP80221,
		"ETH_P_HSR":             EthPHSR,
		"ETH_P_NSH":             EthPNSH,
		"ETH_P_LOOPBACK":        EthPLOOPBACK,
		"ETH_P_QINQ1":           EthPQINQ1,
		"ETH_P_QINQ2":           EthPQINQ2,
		"ETH_P_QINQ3":           EthPQINQ3,
		"ETH_P_EDSA":            EthPEDSA,
		"ETH_P_IFE":             EthPIFE,
		"ETH_P_AFIUCV":          EthPAFIUCV,
		"ETH_P_8023_MIN":        EthP8023MIN,
		"ETH_P_IPV6_HOP_BY_HOP": EthPIPV6HopByHop,
		"ETH_P_8023":            EthP8023,
		"ETH_P_AX25":            EthPAX25,
		"ETH_P_ALL":             EthPALL,
		"ETH_P_8022":            EthP8022,
		"ETH_P_SNAP":            EthPSNAP,
		"ETH_P_DDCMP":           EthPDDCMP,
		"ETH_P_WANPPP":          EthPWANPPP,
		"ETH_P_PPPMP":           EthPPPPMP,
		"ETH_P_LOCALTALK":       EthPLOCALTALK,
		"ETH_P_CAN":             EthPCAN,
		"ETH_P_CANFD":           EthPCANFD,
		"ETH_P_PPPTALK":         EthPPPPTALK,
		"ETH_P_TR8022":          EthPTR8022,
		"ETH_P_MOBITEX":         EthPMOBITEX,
		"ETH_P_CONTROL":         EthPCONTROL,
		"ETH_P_IRDA":            EthPIRDA,
		"ETH_P_ECONET":          EthPECONET,
		"ETH_P_HDLC":            EthPHDLC,
		"ETH_P_ARCNET":          EthPARCNET,
		"ETH_P_DSA":             EthPDSA,
		"ETH_P_TRAILER":         EthPTRAILER,
		"ETH_P_PHONET":          EthPPHONET,
		"ETH_P_IEEE802154":      EthPIEEE802154,
		"ETH_P_CAIF":            EthPCAIF,
		"ETH_P_XDSA":            EthPXDSA,
		"ETH_P_MAP":             EthPMAP,
	}

	// L4ProtocolConstants is the list of supported L4 protocols
	// generate_constants:L4 protocols,L4 protocols are the supported Layer 4 protocols.
	L4ProtocolConstants = map[string]L4Protocol{
		"IP_PROTO_IP":      IPProtoIP,
		"IP_PROTO_ICMP":    IPProtoICMP,
		"IP_PROTO_IGMP":    IPProtoIGMP,
		"IP_PROTO_IPIP":    IPProtoIPIP,
		"IP_PROTO_TCP":     IPProtoTCP,
		"IP_PROTO_EGP":     IPProtoEGP,
		"IP_PROTO_IGP":     IPProtoIGP,
		"IP_PROTO_PUP":     IPProtoPUP,
		"IP_PROTO_UDP":     IPProtoUDP,
		"IP_PROTO_IDP":     IPProtoIDP,
		"IP_PROTO_TP":      IPProtoTP,
		"IP_PROTO_DCCP":    IPProtoDCCP,
		"IP_PROTO_IPV6":    IPProtoIPV6,
		"IP_PROTO_RSVP":    IPProtoRSVP,
		"IP_PROTO_GRE":     IPProtoGRE,
		"IP_PROTO_ESP":     IPProtoESP,
		"IP_PROTO_AH":      IPProtoAH,
		"IP_PROTO_ICMPV6":  IPProtoICMPV6,
		"IP_PROTO_MTP":     IPProtoMTP,
		"IP_PROTO_BEETPH":  IPProtoBEETPH,
		"IP_PROTO_ENCAP":   IPProtoENCAP,
		"IP_PROTO_PIM":     IPProtoPIM,
		"IP_PROTO_COMP":    IPProtoCOMP,
		"IP_PROTO_SCTP":    IPProtoSCTP,
		"IP_PROTO_UDPLITE": IPProtoUDPLITE,
		"IP_PROTO_MPLS":    IPProtoMPLS,
		"IP_PROTO_RAW":     IPProtoRAW,
	}

	// exitCauseConstants is the list of supported Exit causes
	exitCauseConstants = map[string]ExitCause{
		"EXITED":     ExitExited,
		"COREDUMPED": ExitCoreDumped,
		"SIGNALED":   ExitSignaled,
	}
)

var (
	dnsQTypeStrings      = map[uint32]string{}
	dnsQClassStrings     = map[uint32]string{}
	l3ProtocolStrings    = map[L3Protocol]string{}
	l4ProtocolStrings    = map[L4Protocol]string{}
	addressFamilyStrings = map[uint16]string{}
	exitCauseStrings     = map[ExitCause]string{}
)

// File flags
const (
	LowerLayer = 1 << iota
	UpperLayer
)

func initErrorConstants() {
	for k, v := range errorConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
	}
}

func initDNSQClassConstants() {
	for k, v := range DNSQClassConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
		dnsQClassStrings[uint32(v)] = k
	}
}

func initDNSQTypeConstants() {
	for k, v := range DNSQTypeConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: v}
		dnsQTypeStrings[uint32(v)] = k
	}
}

func initL3ProtocolConstants() {
	for k, v := range L3ProtocolConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		l3ProtocolStrings[v] = k
	}
}

func initL4ProtocolConstants() {
	for k, v := range L4ProtocolConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		l4ProtocolStrings[v] = k
	}
}

func initAddressFamilyConstants() {
	for k, v := range addressFamilyConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}

	for k, v := range addressFamilyConstants {
		addressFamilyStrings[v] = k
	}
}

func initExitCauseConstants() {
	for k, v := range exitCauseConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
		exitCauseStrings[v] = k
	}
}

func initBoolConstants() {
	for k, v := range BooleanConstants {
		seclConstants[k] = v
	}
}

func initContainerFlagsConstants() {
	for k, v := range ContainerFlagsConstants {
		seclConstants[k] = &eval.IntEvaluator{Value: int(v)}
	}
}

func initConstants() {
	initBoolConstants()
	initErrorConstants()
	initOpenConstants()
	initFileModeConstants()
	initInodeModeConstants()
	initUnlinkConstanst()
	initKernelCapabilityConstants()
	initBPFCmdConstants()
	initBPFHelperFuncConstants()
	initBPFMapTypeConstants()
	initBPFProgramTypeConstants()
	initBPFAttachTypeConstants()
	initPtraceConstants()
	initVMConstants()
	initProtConstansts()
	initMMapFlagsConstants()
	initSignalConstants()
	initPipeBufFlagConstants()
	initDNSQClassConstants()
	initDNSQTypeConstants()
	initL3ProtocolConstants()
	initL4ProtocolConstants()
	initAddressFamilyConstants()
	initExitCauseConstants()
	initBPFMapNamesConstants()
	usersession.InitUserSessionTypes()
	initContainerFlagsConstants()
}

// RetValError represents a syscall return error value
type RetValError int

func (f RetValError) String() string {
	v := int(f)
	if v < 0 {
		return syscall.Errno(-v).Error()
	}
	return ""
}

var constantsInitialized sync.Once

// SECLConstants returns the constants supported in runtime security agent rules,
// initializing these constants during the first call
func SECLConstants() map[string]interface{} {
	constantsInitialized.Do(func() {
		initConstants()
	})
	return seclConstants
}

// AddressFamily represents a family address (AF_INET, AF_INET6, AF_UNIX etc)
type AddressFamily int

func (af AddressFamily) String() string {
	return addressFamilyStrings[uint16(af)]
}

// QClass is used to declare the qclass field of a DNS request
type QClass uint32

func (qc QClass) String() string {
	if val, ok := dnsQClassStrings[uint32(qc)]; ok {
		return val
	}
	return fmt.Sprintf("qclass(%d)", qc)
}

// QType is used to declare the qtype field of a DNS request
type QType uint32

func (qt QType) String() string {
	if val, ok := dnsQTypeStrings[uint32(qt)]; ok {
		return val
	}
	return fmt.Sprintf("qtype(%d)", qt)
}

// L3Protocol Network protocols
type L3Protocol uint16

func (proto L3Protocol) String() string {
	return l3ProtocolStrings[proto]
}

const (
	// EthPLOOP Ethernet Loopback packet
	EthPLOOP L3Protocol = 0x0060
	// EthPPUP Xerox PUP packet
	EthPPUP L3Protocol = 0x0200
	// EthPPUPAT Xerox PUP Addr Trans packet
	EthPPUPAT L3Protocol = 0x0201
	// EthPTSN TSN (IEEE 1722) packet
	EthPTSN L3Protocol = 0x22F0
	// EthPIP Internet Protocol packet
	EthPIP L3Protocol = 0x0800
	// EthPX25 CCITT X.25
	EthPX25 L3Protocol = 0x0805
	// EthPARP Address Resolution packet
	EthPARP L3Protocol = 0x0806
	// EthPBPQ G8BPQ AX.25 Ethernet Packet    [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPBPQ L3Protocol = 0x08FF
	// EthPIEEEPUP Xerox IEEE802.3 PUP packet
	EthPIEEEPUP L3Protocol = 0x0a00
	// EthPIEEEPUPAT Xerox IEEE802.3 PUP Addr Trans packet
	EthPIEEEPUPAT L3Protocol = 0x0a01
	// EthPBATMAN B.A.T.M.A.N.-Advanced packet [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPBATMAN L3Protocol = 0x4305
	// EthPDEC DEC Assigned proto
	EthPDEC L3Protocol = 0x6000
	// EthPDNADL DEC DNA Dump/Load
	EthPDNADL L3Protocol = 0x6001
	// EthPDNARC DEC DNA Remote Console
	EthPDNARC L3Protocol = 0x6002
	// EthPDNART DEC DNA Routing
	EthPDNART L3Protocol = 0x6003
	// EthPLAT DEC LAT
	EthPLAT L3Protocol = 0x6004
	// EthPDIAG DEC Diagnostics
	EthPDIAG L3Protocol = 0x6005
	// EthPCUST DEC Customer use
	EthPCUST L3Protocol = 0x6006
	// EthPSCA DEC Systems Comms Arch
	EthPSCA L3Protocol = 0x6007
	// EthPTEB Trans Ether Bridging
	EthPTEB L3Protocol = 0x6558
	// EthPRARP Reverse Addr Res packet
	EthPRARP L3Protocol = 0x8035
	// EthPATALK Appletalk DDP
	EthPATALK L3Protocol = 0x809B
	// EthPAARP Appletalk AARP
	EthPAARP L3Protocol = 0x80F3
	// EthP8021Q 802.1Q VLAN Extended Header
	EthP8021Q L3Protocol = 0x8100
	// EthPERSPAN ERSPAN type II
	EthPERSPAN L3Protocol = 0x88BE
	// EthPIPX IPX over DIX
	EthPIPX L3Protocol = 0x8137
	// EthPIPV6 IPv6 over bluebook
	EthPIPV6 L3Protocol = 0x86DD
	// EthPPAUSE IEEE Pause frames. See 802.3 31B
	EthPPAUSE L3Protocol = 0x8808
	// EthPSLOW Slow Protocol. See 802.3ad 43B
	EthPSLOW L3Protocol = 0x8809
	// EthPWCCP Web-cache coordination protocol defined in draft-wilson-wrec-wccp-v2-00.txt
	EthPWCCP L3Protocol = 0x883E
	// EthPMPLSUC MPLS Unicast traffic
	EthPMPLSUC L3Protocol = 0x8847
	// EthPMPLSMC MPLS Multicast traffic
	EthPMPLSMC L3Protocol = 0x8848
	// EthPATMMPOA MultiProtocol Over ATM
	EthPATMMPOA L3Protocol = 0x884c
	// EthPPPPDISC PPPoE discovery messages
	EthPPPPDISC L3Protocol = 0x8863
	// EthPPPPSES PPPoE session messages
	EthPPPPSES L3Protocol = 0x8864
	// EthPLinkCTL HPNA, wlan link local tunnel
	EthPLinkCTL L3Protocol = 0x886c
	// EthPATMFATE Frame-based ATM Transport over Ethernet
	EthPATMFATE L3Protocol = 0x8884
	// EthPPAE Port Access Entity (IEEE 802.1X)
	EthPPAE L3Protocol = 0x888E
	// EthPAOE ATA over Ethernet
	EthPAOE L3Protocol = 0x88A2
	// EthP8021AD 802.1ad Service VLAN
	EthP8021AD L3Protocol = 0x88A8
	// EthP802EX1 802.1 Local Experimental 1.
	EthP802EX1 L3Protocol = 0x88B5
	// EthPTIPC TIPC
	EthPTIPC L3Protocol = 0x88CA
	// EthPMACSEC 802.1ae MACsec
	EthPMACSEC L3Protocol = 0x88E5
	// EthP8021AH 802.1ah Backbone Service Tag
	EthP8021AH L3Protocol = 0x88E7
	// EthPMVRP 802.1Q MVRP
	EthPMVRP L3Protocol = 0x88F5
	// EthP1588 IEEE 1588 Timesync
	EthP1588 L3Protocol = 0x88F7
	// EthPNCSI NCSI protocol
	EthPNCSI L3Protocol = 0x88F8
	// EthPPRP IEC 62439-3 PRP/HSRv0
	EthPPRP L3Protocol = 0x88FB
	// EthPFCOE Fibre Channel over Ethernet
	EthPFCOE L3Protocol = 0x8906
	// EthPIBOE Infiniband over Ethernet
	EthPIBOE L3Protocol = 0x8915
	// EthPTDLS TDLS
	EthPTDLS L3Protocol = 0x890D
	// EthPFIP FCoE Initialization Protocol
	EthPFIP L3Protocol = 0x8914
	// EthP80221 IEEE 802.21 Media Independent Handover Protocol
	EthP80221 L3Protocol = 0x8917
	// EthPHSR IEC 62439-3 HSRv1
	EthPHSR L3Protocol = 0x892F
	// EthPNSH Network Service Header
	EthPNSH L3Protocol = 0x894F
	// EthPLOOPBACK Ethernet loopback packet, per IEEE 802.3
	EthPLOOPBACK L3Protocol = 0x9000
	// EthPQINQ1 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ1 L3Protocol = 0x9100
	// EthPQINQ2 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ2 L3Protocol = 0x9200
	// EthPQINQ3 deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPQINQ3 L3Protocol = 0x9300
	// EthPEDSA Ethertype DSA [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPEDSA L3Protocol = 0xDADA
	// EthPIFE ForCES inter-FE LFB type
	EthPIFE L3Protocol = 0xED3E
	// EthPAFIUCV IBM afiucv [ NOT AN OFFICIALLY REGISTERED ID ]
	EthPAFIUCV L3Protocol = 0xFBFB
	// EthP8023MIN If the value in the ethernet type is less than this value then the frame is Ethernet II. Else it is 802.3
	EthP8023MIN L3Protocol = 0x0600
	// EthPIPV6HopByHop IPv6 Hop by hop option
	EthPIPV6HopByHop L3Protocol = 0x000
	// EthP8023 Dummy type for 802.3 frames
	EthP8023 L3Protocol = 0x0001
	// EthPAX25 Dummy protocol id for AX.25
	EthPAX25 L3Protocol = 0x0002
	// EthPALL Every packet (be careful!!!)
	EthPALL L3Protocol = 0x0003
	// EthP8022 802.2 frames
	EthP8022 L3Protocol = 0x0004
	// EthPSNAP Internal only
	EthPSNAP L3Protocol = 0x0005
	// EthPDDCMP DEC DDCMP: Internal only
	EthPDDCMP L3Protocol = 0x0006
	// EthPWANPPP Dummy type for WAN PPP frames*/
	EthPWANPPP L3Protocol = 0x0007
	// EthPPPPMP Dummy type for PPP MP frames
	EthPPPPMP L3Protocol = 0x0008
	// EthPLOCALTALK Localtalk pseudo type
	EthPLOCALTALK L3Protocol = 0x0009
	// EthPCAN CAN: Controller Area Network
	EthPCAN L3Protocol = 0x000C
	// EthPCANFD CANFD: CAN flexible data rate*/
	EthPCANFD L3Protocol = 0x000D
	// EthPPPPTALK Dummy type for Atalk over PPP*/
	EthPPPPTALK L3Protocol = 0x0010
	// EthPTR8022 802.2 frames
	EthPTR8022 L3Protocol = 0x0011
	// EthPMOBITEX Mobitex (kaz@cafe.net)
	EthPMOBITEX L3Protocol = 0x0015
	// EthPCONTROL Card specific control frames
	EthPCONTROL L3Protocol = 0x0016
	// EthPIRDA Linux-IrDA
	EthPIRDA L3Protocol = 0x0017
	// EthPECONET Acorn Econet
	EthPECONET L3Protocol = 0x0018
	// EthPHDLC HDLC frames
	EthPHDLC L3Protocol = 0x0019
	// EthPARCNET 1A for ArcNet :-)
	EthPARCNET L3Protocol = 0x001A
	// EthPDSA Distributed Switch Arch.
	EthPDSA L3Protocol = 0x001B
	// EthPTRAILER Trailer switch tagging
	EthPTRAILER L3Protocol = 0x001C
	// EthPPHONET Nokia Phonet frames
	EthPPHONET L3Protocol = 0x00F5
	// EthPIEEE802154 IEEE802.15.4 frame
	EthPIEEE802154 L3Protocol = 0x00F6
	// EthPCAIF ST-Ericsson CAIF protocol
	EthPCAIF L3Protocol = 0x00F7
	// EthPXDSA Multiplexed DSA protocol
	EthPXDSA L3Protocol = 0x00F8
	// EthPMAP Qualcomm multiplexing and aggregation protocol
	EthPMAP L3Protocol = 0x00F9
)

// L4Protocol transport protocols
type L4Protocol uint16

func (proto L4Protocol) String() string {
	return l4ProtocolStrings[proto]
}

const (
	// IPProtoIP Dummy protocol for TCP
	IPProtoIP L4Protocol = 0
	// IPProtoICMP Internet Control Message Protocol (IPv4)
	IPProtoICMP L4Protocol = 1
	// IPProtoIGMP Internet Group Management Protocol
	IPProtoIGMP L4Protocol = 2
	// IPProtoIPIP IPIP tunnels (older KA9Q tunnels use 94)
	IPProtoIPIP L4Protocol = 4
	// IPProtoTCP Transmission Control Protocol
	IPProtoTCP L4Protocol = 6
	// IPProtoEGP Exterior Gateway Protocol
	IPProtoEGP L4Protocol = 8
	// IPProtoIGP Interior Gateway Protocol (any private interior gateway (used by Cisco for their IGRP))
	IPProtoIGP L4Protocol = 9
	// IPProtoPUP PUP protocol
	IPProtoPUP L4Protocol = 12
	// IPProtoUDP User Datagram Protocol
	IPProtoUDP L4Protocol = 17
	// IPProtoIDP XNS IDP protocol
	IPProtoIDP L4Protocol = 22
	// IPProtoTP SO Transport Protocol Class 4
	IPProtoTP L4Protocol = 29
	// IPProtoDCCP Datagram Congestion Control Protocol
	IPProtoDCCP L4Protocol = 33
	// IPProtoIPV6 IPv6-in-IPv4 tunnelling
	IPProtoIPV6 L4Protocol = 41
	// IPProtoRSVP RSVP Protocol
	IPProtoRSVP L4Protocol = 46
	// IPProtoGRE Cisco GRE tunnels (rfc 1701,1702)
	IPProtoGRE L4Protocol = 47
	// IPProtoESP Encapsulation Security Payload protocol
	IPProtoESP L4Protocol = 50
	// IPProtoAH Authentication Header protocol
	IPProtoAH L4Protocol = 51
	// IPProtoICMPV6 Internet Control Message Protocol (IPv6)
	IPProtoICMPV6 L4Protocol = 58
	// IPProtoMTP Multicast Transport Protocol
	IPProtoMTP L4Protocol = 92
	// IPProtoBEETPH IP option pseudo header for BEET
	IPProtoBEETPH L4Protocol = 94
	// IPProtoENCAP Encapsulation Header
	IPProtoENCAP L4Protocol = 98
	// IPProtoPIM Protocol Independent Multicast
	IPProtoPIM L4Protocol = 103
	// IPProtoCOMP Compression Header Protocol
	IPProtoCOMP L4Protocol = 108
	// IPProtoSCTP Stream Control Transport Protocol
	IPProtoSCTP L4Protocol = 132
	// IPProtoUDPLITE UDP-Lite (RFC 3828)
	IPProtoUDPLITE L4Protocol = 136
	// IPProtoMPLS MPLS in IP (RFC 4023)
	IPProtoMPLS L4Protocol = 137
	// IPProtoRAW Raw IP packets
	IPProtoRAW L4Protocol = 255
)

// ExitCause represents the cause of a process termination
type ExitCause uint32

func (cause ExitCause) String() string {
	return exitCauseStrings[cause]
}

const (
	// ExitExited Process exited normally
	ExitExited ExitCause = iota
	// ExitCoreDumped Process was terminated with a coredump signal
	ExitCoreDumped
	// ExitSignaled Process was terminated with a signal other than a coredump
	ExitSignaled
)

// CGroup managers
const (
	CGroupManagerDocker uint64 = iota + 1
	CGroupManagerCRIO
	CGroupManagerPodman
	CGroupManagerCRI
	CGroupManagerSystemd
)

var (
	// ContainerFlagsConstants are the supported flags for a container
	// generate_constants:ContainerFlags flags,Container flags are the supported flags for a container.
	ContainerFlagsConstants = map[string]uint64{
		"MANAGED_BY_DOCKER":         CGroupManagerDocker,
		"MANAGED_BY_CRI_CONTAINERD": CGroupManagerCRI,
		"MANAGED_BY_CRIO":           CGroupManagerCRIO,
		"MANAGED_BY_PODMAN":         CGroupManagerPodman,
		"MANAGED_BY_SYSTEMD":        CGroupManagerSystemd,
	}
)
