package probe

import "fmt"

// TransportProtocol - Transport protocols
type TransportProtocol int64

const (
	IPPROTO_IP      TransportProtocol = 0   /* Dummy protocol for TCP		*/
	IPPROTO_ICMP    TransportProtocol = 1   /* Internet Control Message Protocol (IPv4) */
	IPPROTO_IGMP    TransportProtocol = 2   /* Internet Group Management Protocol	*/
	IPPROTO_IPIP    TransportProtocol = 4   /* IPIP tunnels (older KA9Q tunnels use 94) */
	IPPROTO_TCP     TransportProtocol = 6   /* Transmission Control Protocol	*/
	IPPROTO_EGP     TransportProtocol = 8   /* Exterior Gateway Protocol		*/
	IPPROTO_IGP     TransportProtocol = 9   /* Interior Gateway Protocol (any private interior gateway (used by Cisco for their IGRP)) */
	IPPROTO_PUP     TransportProtocol = 12  /* PUP protocol				*/
	IPPROTO_UDP     TransportProtocol = 17  /* User Datagram Protocol		*/
	IPPROTO_IDP     TransportProtocol = 22  /* XNS IDP protocol			*/
	IPPROTO_TP      TransportProtocol = 29  /* SO Transport Protocol Class 4	*/
	IPPROTO_DCCP    TransportProtocol = 33  /* Datagram Congestion Control Protocol */
	IPPROTO_IPV6    TransportProtocol = 41  /* IPv6-in-IPv4 tunnelling		*/
	IPPROTO_RSVP    TransportProtocol = 46  /* RSVP Protocol			*/
	IPPROTO_GRE     TransportProtocol = 47  /* Cisco GRE tunnels (rfc 1701,1702)	*/
	IPPROTO_ESP     TransportProtocol = 50  /* Encapsulation Security Payload protocol */
	IPPROTO_AH      TransportProtocol = 51  /* Authentication Header protocol	*/
	IPPROTO_ICMPV6  TransportProtocol = 58  /* Internet Control Message Protocol (IPv6)	*/
	IPPROTO_MTP     TransportProtocol = 92  /* Multicast Transport Protocol		*/
	IPPROTO_BEETPH  TransportProtocol = 94  /* IP option pseudo header for BEET	*/
	IPPROTO_ENCAP   TransportProtocol = 98  /* Encapsulation Header			*/
	IPPROTO_PIM     TransportProtocol = 103 /* Protocol Independent Multicast	*/
	IPPROTO_COMP    TransportProtocol = 108 /* Compression Header Protocol		*/
	IPPROTO_SCTP    TransportProtocol = 132 /* Stream Control Transport Protocol	*/
	IPPROTO_UDPLITE TransportProtocol = 136 /* UDP-Lite (RFC 3828)			*/
	IPPROTO_MPLS    TransportProtocol = 137 /* MPLS in IP (RFC 4023)		*/
	IPPROTO_RAW     TransportProtocol = 255 /* Raw IP packets			*/
)

// TransportProtocolToString - Returns the string representation of a TransportProtocol
func TransportProtocolToString(tp int64) string {
	switch TransportProtocol(tp) {
	case IPPROTO_IP:
		return "IPPROTO_IP"
	case IPPROTO_ICMP:
		return "IPPROTO_ICMP"
	case IPPROTO_IGMP:
		return "IPPROTO_IGMP"
	case IPPROTO_IPIP:
		return "IPPROTO_IPIP"
	case IPPROTO_TCP:
		return "IPPROTO_TCP"
	case IPPROTO_EGP:
		return "IPPROTO_EGP"
	case IPPROTO_IGP:
		return "IPPROTO_IGP"
	case IPPROTO_PUP:
		return "IPPROTO_PUP"
	case IPPROTO_UDP:
		return "IPPROTO_UDP"
	case IPPROTO_IDP:
		return "IPPROTO_IDP"
	case IPPROTO_TP:
		return "IPPROTO_TP"
	case IPPROTO_DCCP:
		return "IPPROTO_DCCP"
	case IPPROTO_IPV6:
		return "IPPROTO_IPV6"
	case IPPROTO_RSVP:
		return "IPPROTO_RSVP"
	case IPPROTO_GRE:
		return "IPPROTO_GRE"
	case IPPROTO_ESP:
		return "IPPROTO_ESP"
	case IPPROTO_AH:
		return "IPPROTO_AH"
	case IPPROTO_ICMPV6:
		return "IPPROTO_ICMPV6"
	case IPPROTO_MTP:
		return "IPPROTO_MTP"
	case IPPROTO_BEETPH:
		return "IPPROTO_BEETPH"
	case IPPROTO_ENCAP:
		return "IPPROTO_ENCAP"
	case IPPROTO_PIM:
		return "IPPROTO_PIM"
	case IPPROTO_COMP:
		return "IPPROTO_COMP"
	case IPPROTO_SCTP:
		return "IPPROTO_SCTP"
	case IPPROTO_UDPLITE:
		return "IPPROTO_UDPLITE"
	case IPPROTO_MPLS:
		return "IPPROTO_MPLS"
	case IPPROTO_RAW:
		return "IPPROTO_RAW"
	default:
		return fmt.Sprintf("TransportProtocol(%v)", tp)
	}
}

// NetworkProtocol - Network protocols
type NetworkProtocol uint64

const (
	ETH_P_LOOP            NetworkProtocol = 0x0060 /* Ethernet Loopback packet	*/
	ETH_P_PUP             NetworkProtocol = 0x0200 /* Xerox PUP packet		*/
	ETH_P_PUPAT           NetworkProtocol = 0x0201 /* Xerox PUP Addr Trans packet	*/
	ETH_P_TSN             NetworkProtocol = 0x22F0 /* TSN (IEEE 1722) packet	*/
	ETH_P_IP              NetworkProtocol = 0x0800 /* Internet Protocol packet	*/
	ETH_P_X25             NetworkProtocol = 0x0805 /* CCITT X.25			*/
	ETH_P_ARP             NetworkProtocol = 0x0806 /* Address Resolution packet	*/
	ETH_P_BPQ             NetworkProtocol = 0x08FF /* G8BPQ AX.25 Ethernet Packet	[ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_IEEEPUP         NetworkProtocol = 0x0a00 /* Xerox IEEE802.3 PUP packet */
	ETH_P_IEEEPUPAT       NetworkProtocol = 0x0a01 /* Xerox IEEE802.3 PUP Addr Trans packet */
	ETH_P_BATMAN          NetworkProtocol = 0x4305 /* B.A.T.M.A.N.-Advanced packet [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_DEC             NetworkProtocol = 0x6000 /* DEC Assigned proto           */
	ETH_P_DNA_DL          NetworkProtocol = 0x6001 /* DEC DNA Dump/Load            */
	ETH_P_DNA_RC          NetworkProtocol = 0x6002 /* DEC DNA Remote Console       */
	ETH_P_DNA_RT          NetworkProtocol = 0x6003 /* DEC DNA Routing              */
	ETH_P_LAT             NetworkProtocol = 0x6004 /* DEC LAT                      */
	ETH_P_DIAG            NetworkProtocol = 0x6005 /* DEC Diagnostics              */
	ETH_P_CUST            NetworkProtocol = 0x6006 /* DEC Customer use             */
	ETH_P_SCA             NetworkProtocol = 0x6007 /* DEC Systems Comms Arch       */
	ETH_P_TEB             NetworkProtocol = 0x6558 /* Trans Ether Bridging		*/
	ETH_P_RARP            NetworkProtocol = 0x8035 /* Reverse Addr Res packet	*/
	ETH_P_ATALK           NetworkProtocol = 0x809B /* Appletalk DDP		*/
	ETH_P_AARP            NetworkProtocol = 0x80F3 /* Appletalk AARP		*/
	ETH_P_8021Q           NetworkProtocol = 0x8100 /* 802.1Q VLAN Extended Header  */
	ETH_P_ERSPAN          NetworkProtocol = 0x88BE /* ERSPAN type II		*/
	ETH_P_IPX             NetworkProtocol = 0x8137 /* IPX over DIX			*/
	ETH_P_IPV6            NetworkProtocol = 0x86DD /* IPv6 over bluebook		*/
	ETH_P_PAUSE           NetworkProtocol = 0x8808 /* IEEE Pause frames. See 802.3 31B */
	ETH_P_SLOW            NetworkProtocol = 0x8809 /* Slow Protocol. See 802.3ad 43B */
	ETH_P_WCCP            NetworkProtocol = 0x883E /* Web-cache coordination protocol defined in draft-wilson-wrec-wccp-v2-00.txt */
	ETH_P_MPLS_UC         NetworkProtocol = 0x8847 /* MPLS Unicast traffic		*/
	ETH_P_MPLS_MC         NetworkProtocol = 0x8848 /* MPLS Multicast traffic	*/
	ETH_P_ATMMPOA         NetworkProtocol = 0x884c /* MultiProtocol Over ATM	*/
	ETH_P_PPP_DISC        NetworkProtocol = 0x8863 /* PPPoE discovery messages     */
	ETH_P_PPP_SES         NetworkProtocol = 0x8864 /* PPPoE session messages	*/
	ETH_P_LINK_CTL        NetworkProtocol = 0x886c /* HPNA, wlan link local tunnel */
	ETH_P_ATMFATE         NetworkProtocol = 0x8884 /* Frame-based ATM Transport over Ethernet */
	ETH_P_PAE             NetworkProtocol = 0x888E /* Port Access Entity (IEEE 802.1X) */
	ETH_P_AOE             NetworkProtocol = 0x88A2 /* ATA over Ethernet		*/
	ETH_P_8021AD          NetworkProtocol = 0x88A8 /* 802.1ad Service VLAN		*/
	ETH_P_802_EX1         NetworkProtocol = 0x88B5 /* 802.1 Local Experimental 1.  */
	ETH_P_TIPC            NetworkProtocol = 0x88CA /* TIPC 			*/
	ETH_P_MACSEC          NetworkProtocol = 0x88E5 /* 802.1ae MACsec */
	ETH_P_8021AH          NetworkProtocol = 0x88E7 /* 802.1ah Backbone Service Tag */
	ETH_P_MVRP            NetworkProtocol = 0x88F5 /* 802.1Q MVRP                  */
	ETH_P_1588            NetworkProtocol = 0x88F7 /* IEEE 1588 Timesync */
	ETH_P_NCSI            NetworkProtocol = 0x88F8 /* NCSI protocol		*/
	ETH_P_PRP             NetworkProtocol = 0x88FB /* IEC 62439-3 PRP/HSRv0	*/
	ETH_P_FCOE            NetworkProtocol = 0x8906 /* Fibre Channel over Ethernet  */
	ETH_P_IBOE            NetworkProtocol = 0x8915 /* Infiniband over Ethernet	*/
	ETH_P_TDLS            NetworkProtocol = 0x890D /* TDLS */
	ETH_P_FIP             NetworkProtocol = 0x8914 /* FCoE Initialization Protocol */
	ETH_P_80221           NetworkProtocol = 0x8917 /* IEEE 802.21 Media Independent Handover Protocol */
	ETH_P_HSR             NetworkProtocol = 0x892F /* IEC 62439-3 HSRv1	*/
	ETH_P_NSH             NetworkProtocol = 0x894F /* Network Service Header */
	ETH_P_LOOPBACK        NetworkProtocol = 0x9000 /* Ethernet loopback packet, per IEEE 802.3 */
	ETH_P_QINQ1           NetworkProtocol = 0x9100 /* deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_QINQ2           NetworkProtocol = 0x9200 /* deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_QINQ3           NetworkProtocol = 0x9300 /* deprecated QinQ VLAN [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_EDSA            NetworkProtocol = 0xDADA /* Ethertype DSA [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_IFE             NetworkProtocol = 0xED3E /* ForCES inter-FE LFB type */
	ETH_P_AF_IUCV         NetworkProtocol = 0xFBFB /* IBM af_iucv [ NOT AN OFFICIALLY REGISTERED ID ] */
	ETH_P_802_3_MIN       NetworkProtocol = 0x0600 /* If the value in the ethernet type is less than this value then the frame is Ethernet II. Else it is 802.3 */
	ETH_P_IPV6_HOP_BY_HOP NetworkProtocol = 0x000  /* IPv6 Hop by hop option */
	ETH_P_802_3           NetworkProtocol = 0x0001 /* Dummy type for 802.3 frames  */
	ETH_P_AX25            NetworkProtocol = 0x0002 /* Dummy protocol id for AX.25  */
	ETH_P_ALL             NetworkProtocol = 0x0003 /* Every packet (be careful!!!) */
	ETH_P_802_2           NetworkProtocol = 0x0004 /* 802.2 frames 		*/
	ETH_P_SNAP            NetworkProtocol = 0x0005 /* Internal only		*/
	ETH_P_DDCMP           NetworkProtocol = 0x0006 /* DEC DDCMP: Internal only     */
	ETH_P_WAN_PPP         NetworkProtocol = 0x0007 /* Dummy type for WAN PPP frames*/
	ETH_P_PPP_MP          NetworkProtocol = 0x0008 /* Dummy type for PPP MP frames */
	ETH_P_LOCALTALK       NetworkProtocol = 0x0009 /* Localtalk pseudo type 	*/
	ETH_P_CAN             NetworkProtocol = 0x000C /* CAN: Controller Area Network */
	ETH_P_CANFD           NetworkProtocol = 0x000D /* CANFD: CAN flexible data rate*/
	ETH_P_PPPTALK         NetworkProtocol = 0x0010 /* Dummy type for Atalk over PPP*/
	ETH_P_TR_802_2        NetworkProtocol = 0x0011 /* 802.2 frames 		*/
	ETH_P_MOBITEX         NetworkProtocol = 0x0015 /* Mobitex (kaz@cafe.net)	*/
	ETH_P_CONTROL         NetworkProtocol = 0x0016 /* Card specific control frames */
	ETH_P_IRDA            NetworkProtocol = 0x0017 /* Linux-IrDA			*/
	ETH_P_ECONET          NetworkProtocol = 0x0018 /* Acorn Econet			*/
	ETH_P_HDLC            NetworkProtocol = 0x0019 /* HDLC frames			*/
	ETH_P_ARCNET          NetworkProtocol = 0x001A /* 1A for ArcNet :-)            */
	ETH_P_DSA             NetworkProtocol = 0x001B /* Distributed Switch Arch.	*/
	ETH_P_TRAILER         NetworkProtocol = 0x001C /* Trailer switch tagging	*/
	ETH_P_PHONET          NetworkProtocol = 0x00F5 /* Nokia Phonet frames          */
	ETH_P_IEEE802154      NetworkProtocol = 0x00F6 /* IEEE802.15.4 frame		*/
	ETH_P_CAIF            NetworkProtocol = 0x00F7 /* ST-Ericsson CAIF protocol	*/
	ETH_P_XDSA            NetworkProtocol = 0x00F8 /* Multiplexed DSA protocol	*/
	ETH_P_MAP             NetworkProtocol = 0x00F9 /* Qualcomm multiplexing and aggregation protocol */
)

// NetworkProtocolToString - Returns the string representation of a NetworkProtocol
func NetworkProtocolToString(np uint64) string {
	switch NetworkProtocol(np) {
	case ETH_P_LOOP:
		return "ETH_P_LOOP"
	case ETH_P_PUP:
		return "ETH_P_PUP"
	case ETH_P_PUPAT:
		return "ETH_P_PUPAT"
	case ETH_P_TSN:
		return "ETH_P_TSN"
	case ETH_P_IP:
		return "ETH_P_IP"
	case ETH_P_X25:
		return "ETH_P_X25"
	case ETH_P_ARP:
		return "ETH_P_ARP"
	case ETH_P_BPQ:
		return "ETH_P_BPQ"
	case ETH_P_IEEEPUP:
		return "ETH_P_IEEEPUP"
	case ETH_P_IEEEPUPAT:
		return "ETH_P_IEEEPUPAT"
	case ETH_P_BATMAN:
		return "ETH_P_BATMAN"
	case ETH_P_DEC:
		return "ETH_P_DEC"
	case ETH_P_DNA_DL:
		return "ETH_P_DNA_DL"
	case ETH_P_DNA_RC:
		return "ETH_P_DNA_RC"
	case ETH_P_DNA_RT:
		return "ETH_P_DNA_RT"
	case ETH_P_LAT:
		return "ETH_P_LAT"
	case ETH_P_DIAG:
		return "ETH_P_DIAG"
	case ETH_P_CUST:
		return "ETH_P_CUST"
	case ETH_P_SCA:
		return "ETH_P_SCA"
	case ETH_P_TEB:
		return "ETH_P_TEB"
	case ETH_P_RARP:
		return "ETH_P_RARP"
	case ETH_P_ATALK:
		return "ETH_P_ATALK"
	case ETH_P_AARP:
		return "ETH_P_AARP"
	case ETH_P_8021Q:
		return "ETH_P_8021Q"
	case ETH_P_ERSPAN:
		return "ETH_P_ERSPAN"
	case ETH_P_IPX:
		return "ETH_P_IPX"
	case ETH_P_IPV6:
		return "ETH_P_IPV6"
	case ETH_P_PAUSE:
		return "ETH_P_PAUSE"
	case ETH_P_SLOW:
		return "ETH_P_SLOW"
	case ETH_P_WCCP:
		return "ETH_P_WCCP"
	case ETH_P_MPLS_UC:
		return "ETH_P_MPLS_UC"
	case ETH_P_MPLS_MC:
		return "ETH_P_MPLS_MC"
	case ETH_P_ATMMPOA:
		return "ETH_P_ATMMPOA"
	case ETH_P_PPP_DISC:
		return "ETH_P_PPP_DISC"
	case ETH_P_PPP_SES:
		return "ETH_P_PPP_SES"
	case ETH_P_LINK_CTL:
		return "ETH_P_LINK_CTL"
	case ETH_P_ATMFATE:
		return "ETH_P_ATMFATE"
	case ETH_P_PAE:
		return "ETH_P_PAE"
	case ETH_P_AOE:
		return "ETH_P_AOE"
	case ETH_P_8021AD:
		return "ETH_P_8021AD"
	case ETH_P_802_EX1:
		return "ETH_P_802_EX1"
	case ETH_P_TIPC:
		return "ETH_P_TIPC"
	case ETH_P_MACSEC:
		return "ETH_P_MACSEC"
	case ETH_P_8021AH:
		return "ETH_P_8021AH"
	case ETH_P_MVRP:
		return "ETH_P_MVRP"
	case ETH_P_1588:
		return "ETH_P_1588"
	case ETH_P_NCSI:
		return "ETH_P_NCSI"
	case ETH_P_PRP:
		return "ETH_P_PRP"
	case ETH_P_FCOE:
		return "ETH_P_FCOE"
	case ETH_P_IBOE:
		return "ETH_P_IBOE"
	case ETH_P_TDLS:
		return "ETH_P_TDLS"
	case ETH_P_FIP:
		return "ETH_P_FIP"
	case ETH_P_80221:
		return "ETH_P_80221"
	case ETH_P_HSR:
		return "ETH_P_HSR"
	case ETH_P_NSH:
		return "ETH_P_NSH"
	case ETH_P_LOOPBACK:
		return "ETH_P_LOOPBACK"
	case ETH_P_QINQ1:
		return "ETH_P_QINQ1"
	case ETH_P_QINQ2:
		return "ETH_P_QINQ2"
	case ETH_P_QINQ3:
		return "ETH_P_QINQ3"
	case ETH_P_EDSA:
		return "ETH_P_EDSA"
	case ETH_P_IFE:
		return "ETH_P_IFE"
	case ETH_P_AF_IUCV:
		return "ETH_P_AF_IUCV"
	case ETH_P_802_3_MIN:
		return "ETH_P_802_3_MIN"
	case ETH_P_IPV6_HOP_BY_HOP:
		return "ETH_P_IPV6_HOP_BY_HOP"
	case ETH_P_802_3:
		return "ETH_P_802_3"
	case ETH_P_AX25:
		return "ETH_P_AX25"
	case ETH_P_ALL:
		return "ETH_P_ALL"
	case ETH_P_802_2:
		return "ETH_P_802_2"
	case ETH_P_SNAP:
		return "ETH_P_SNAP"
	case ETH_P_DDCMP:
		return "ETH_P_DDCMP"
	case ETH_P_WAN_PPP:
		return "ETH_P_WAN_PPP"
	case ETH_P_PPP_MP:
		return "ETH_P_PPP_MP"
	case ETH_P_LOCALTALK:
		return "ETH_P_LOCALTALK"
	case ETH_P_CAN:
		return "ETH_P_CAN"
	case ETH_P_CANFD:
		return "ETH_P_CANFD"
	case ETH_P_PPPTALK:
		return "ETH_P_PPPTALK"
	case ETH_P_TR_802_2:
		return "ETH_P_TR_802_2"
	case ETH_P_MOBITEX:
		return "ETH_P_MOBITEX"
	case ETH_P_CONTROL:
		return "ETH_P_CONTROL"
	case ETH_P_IRDA:
		return "ETH_P_IRDA"
	case ETH_P_ECONET:
		return "ETH_P_ECONET"
	case ETH_P_HDLC:
		return "ETH_P_HDLC"
	case ETH_P_ARCNET:
		return "ETH_P_ARCNET"
	case ETH_P_DSA:
		return "ETH_P_DSA"
	case ETH_P_TRAILER:
		return "ETH_P_TRAILER"
	case ETH_P_PHONET:
		return "ETH_P_PHONET"
	case ETH_P_IEEE802154:
		return "ETH_P_IEEE802154"
	case ETH_P_CAIF:
		return "ETH_P_CAIF"
	case ETH_P_XDSA:
		return "ETH_P_XDSA"
	case ETH_P_MAP:
		return "ETH_P_MAP"
	default:
		return fmt.Sprintf("NetworkProtocol(%v)", np)
	}
}

// TCPFlag - Flags of a TCP packet
type TCPFlag int64

const (
	CWR TCPFlag = 1 << 7
	ECE TCPFlag = 1 << 6
	URG TCPFlag = 1 << 5
	ACK TCPFlag = 1 << 4
	PSH TCPFlag = 1 << 3
	RST TCPFlag = 1 << 2
	SYN TCPFlag = 1 << 1
	FIN TCPFlag = 1
)

// TCPFLagsToStrings - Returns the string list version of flags
func TCPFLagsToStrings(input uint64) []string {
	flags := TCPFlag(input)
	rep := []string{}
	if flags&CWR == CWR {
		rep = append(rep, "CWR")
	}
	if flags&ECE == ECE {
		rep = append(rep, "ECE")
	}
	if flags&URG == URG {
		rep = append(rep, "URG")
	}
	if flags&ACK == ACK {
		rep = append(rep, "ACK")
	}
	if flags&PSH == PSH {
		rep = append(rep, "PSH")
	}
	if flags&RST == RST {
		rep = append(rep, "RST")
	}
	if flags&SYN == SYN {
		rep = append(rep, "SYN")
	}
	if flags&FIN == FIN {
		rep = append(rep, "FIN")
	}
	return rep
}

// ICMPFlag - Flags of an ICMP packet
type ICMPFlag int64

const (
	EchoReply              ICMPFlag = 0
	DestinationUnreachable ICMPFlag = 3
	SourceQuench           ICMPFlag = 4
	RedirectMessage        ICMPFlag = 5
	Echo                   ICMPFlag = 8
	RouterAdvertisement    ICMPFlag = 9
	RouterSolicitation     ICMPFlag = 10
	TimeExceeded           ICMPFlag = 11
	ParameterProblem       ICMPFlag = 12
	Timestamp              ICMPFlag = 13
	TimestampReply         ICMPFlag = 14
	InformationRequest     ICMPFlag = 15
	InformationReply       ICMPFlag = 16
	AddressMaskRequest     ICMPFlag = 17
	AddressMaskReply       ICMPFlag = 18
	Traceroute             ICMPFlag = 30
	ExtendedEchoRequest    ICMPFlag = 42
	ExtendedEchoReply      ICMPFlag = 43
)

// ICMPFlagToString - Returns the string version of an ICMP flag
func ICMPFlagToString(flag uint64) string {
	switch ICMPFlag(flag) {
	case EchoReply:
		return "EchoReply"
	case DestinationUnreachable:
		return "DestinationUnreachable"
	case SourceQuench:
		return "SourceQuench"
	case RedirectMessage:
		return "RedirectMessage"
	case Echo:
		return "Echo"
	case RouterAdvertisement:
		return "RouterAdvertisement"
	case RouterSolicitation:
		return "RouterSolicitation"
	case TimeExceeded:
		return "TimeExceeded"
	case ParameterProblem:
		return "ParameterProblem"
	case Timestamp:
		return "Timestamp"
	case TimestampReply:
		return "TimestampReply"
	case InformationRequest:
		return "InformationRequest"
	case InformationReply:
		return "InformationReply"
	case AddressMaskRequest:
		return "AddressMaskRequest"
	case AddressMaskReply:
		return "AddressMaskReply"
	case Traceroute:
		return "Traceroute"
	case ExtendedEchoRequest:
		return "ExtendedEchoRequest"
	case ExtendedEchoReply:
		return "ExtendedEchoReply"
	default:
		return fmt.Sprintf("ICMP(%v)", flag)
	}
}

type SignalInfo int32

const (
	SIGHUP    SignalInfo = 1
	SIGINT    SignalInfo = 2
	SIGQUIT   SignalInfo = 3
	SIGILL    SignalInfo = 4
	SIGTRAP   SignalInfo = 5
	SIGABRT   SignalInfo = 6
	SIGBUS    SignalInfo = 7
	SIGFPE    SignalInfo = 8
	SIGKILL   SignalInfo = 9
	SIGUSR1   SignalInfo = 10
	SIGSEGV   SignalInfo = 11
	SIGUSR2   SignalInfo = 12
	SIGPIPE   SignalInfo = 13
	SIGALRM   SignalInfo = 14
	SIGTERM   SignalInfo = 15
	SIGSTKFLT SignalInfo = 16
	SIGCHLD   SignalInfo = 17
	SIGCONT   SignalInfo = 18
	SIGSTOP   SignalInfo = 19
	SIGTSTP   SignalInfo = 20
	SIGTTIN   SignalInfo = 21
	SIGTTOU   SignalInfo = 22
	SIGURG    SignalInfo = 23
	SIGXCPU   SignalInfo = 24
	SIGXFSZ   SignalInfo = 25
	SIGVTALRM SignalInfo = 26
	SIGPROF   SignalInfo = 27
	SIGWINCH  SignalInfo = 28
	SIGIO     SignalInfo = 29
	SIGPWR    SignalInfo = 30
	SIGSYS    SignalInfo = 31
)

// SignalInfoToString - Returns a signal as its string representation
func SignalInfoToString(input int32) string {
	si := SignalInfo(input)
	switch si {
	case SIGHUP:
		return "SIGHUP"
	case SIGINT:
		return "SIGINT"
	case SIGQUIT:
		return "SIGQUIT"
	case SIGILL:
		return "SIGILL"
	case SIGTRAP:
		return "SIGTRAP"
	case SIGABRT:
		return "SIGABRT"
	case SIGBUS:
		return "SIGBUS"
	case SIGFPE:
		return "SIGFPE"
	case SIGKILL:
		return "SIGKILL"
	case SIGUSR1:
		return "SIGUSR1"
	case SIGSEGV:
		return "SIGSEGV"
	case SIGUSR2:
		return "SIGUSR2"
	case SIGPIPE:
		return "SIGPIPE"
	case SIGALRM:
		return "SIGALRM"
	case SIGTERM:
		return "SIGTERM"
	case SIGSTKFLT:
		return "SIGSTKFLT"
	case SIGCHLD:
		return "SIGCHLD"
	case SIGCONT:
		return "SIGCONT"
	case SIGSTOP:
		return "SIGSTOP"
	case SIGTSTP:
		return "SIGTSTP"
	case SIGTTIN:
		return "SIGTTIN"
	case SIGTTOU:
		return "SIGTTOU"
	case SIGURG:
		return "SIGURG"
	case SIGXCPU:
		return "SIGXCPU"
	case SIGXFSZ:
		return "SIGXFSZ"
	case SIGVTALRM:
		return "SIGVTALRM"
	case SIGPROF:
		return "SIGPROF"
	case SIGWINCH:
		return "SIGWINCH"
	case SIGIO:
		return "SIGIO"
	case SIGPWR:
		return "SIGPWR"
	case SIGSYS:
		return "SIGSYS"
	default:
		return fmt.Sprintf("SignalInfo(%v)", si)
	}
}

// SocketFamily - Socket family enum
type SocketFamily int32

const (
	AF_UNSPEC     SocketFamily = 0
	AF_UNIX       SocketFamily = 1
	AF_LOCAL      SocketFamily = AF_UNIX
	AF_INET       SocketFamily = 2
	AF_AX25       SocketFamily = 3
	AF_IPX        SocketFamily = 4
	AF_APPLETALK  SocketFamily = 5
	AF_NETROM     SocketFamily = 6
	AF_BRIDGE     SocketFamily = 7
	AF_ATMPVC     SocketFamily = 8
	AF_X25        SocketFamily = 9
	AF_INET6      SocketFamily = 10
	AF_ROSE       SocketFamily = 11
	AF_DECnet     SocketFamily = 12
	AF_NETBEUI    SocketFamily = 13
	AF_SECURITY   SocketFamily = 14
	AF_KEY        SocketFamily = 15
	AF_NETLINK    SocketFamily = 16
	AF_ROUTE      SocketFamily = AF_NETLINK
	AF_PACKET     SocketFamily = 17
	AF_ASH        SocketFamily = 18
	AF_ECONET     SocketFamily = 19
	AF_ATMSVC     SocketFamily = 20
	AF_RDS        SocketFamily = 21
	AF_SNA        SocketFamily = 22
	AF_IRDA       SocketFamily = 23
	AF_PPPOX      SocketFamily = 24
	AF_WANPIPE    SocketFamily = 25
	AF_LLC        SocketFamily = 26
	AF_IB         SocketFamily = 27
	AF_MPLS       SocketFamily = 28
	AF_CAN        SocketFamily = 29
	AF_TIPC       SocketFamily = 30
	AF_BLUETOOTH  SocketFamily = 31
	AF_IUCV       SocketFamily = 32
	AF_RXRPC      SocketFamily = 33
	AF_ISDN       SocketFamily = 34
	AF_PHONET     SocketFamily = 35
	AF_IEEE802154 SocketFamily = 36
	AF_CAIF       SocketFamily = 37
	AF_ALG        SocketFamily = 38
	AF_NFC        SocketFamily = 39
	AF_VSOCK      SocketFamily = 40
	AF_KCM        SocketFamily = 41
	AF_QIPCRTR    SocketFamily = 42
	AF_SMC        SocketFamily = 43
	AF_XDP        SocketFamily = 44
	AF_MAX        SocketFamily = 45
)

// SocketFamilyToString - Returns a socket family as its string representation
func SocketFamilyToString(input int32) string {
	se := SocketFamily(input)
	switch se {
	case AF_UNSPEC:
		return "AF_UNSPEC"
	case AF_UNIX:
		return "AF_UNIX"
	case AF_INET:
		return "AF_INET"
	case AF_AX25:
		return "AF_AX25"
	case AF_IPX:
		return "AF_IPX"
	case AF_APPLETALK:
		return "AF_APPLETALK"
	case AF_NETROM:
		return "AF_NETROM"
	case AF_BRIDGE:
		return "AF_BRIDGE"
	case AF_ATMPVC:
		return "AF_ATMPVC"
	case AF_X25:
		return "AF_X25"
	case AF_INET6:
		return "AF_INET6"
	case AF_ROSE:
		return "AF_ROSE"
	case AF_DECnet:
		return "AF_DECnet"
	case AF_NETBEUI:
		return "AF_NETBEUI"
	case AF_SECURITY:
		return "AF_SECURITY"
	case AF_KEY:
		return "AF_KEY"
	case AF_NETLINK:
		return "AF_NETLINK"
	case AF_PACKET:
		return "AF_PACKET"
	case AF_ASH:
		return "AF_ASH"
	case AF_ECONET:
		return "AF_ECONET"
	case AF_ATMSVC:
		return "AF_ATMSVC"
	case AF_RDS:
		return "AF_RDS"
	case AF_SNA:
		return "AF_SNA"
	case AF_IRDA:
		return "AF_IRDA"
	case AF_PPPOX:
		return "AF_PPPOX"
	case AF_WANPIPE:
		return "AF_WANPIPE"
	case AF_LLC:
		return "AF_LLC"
	case AF_IB:
		return "AF_IB"
	case AF_MPLS:
		return "AF_MPLS"
	case AF_CAN:
		return "AF_CAN"
	case AF_TIPC:
		return "AF_TIPC"
	case AF_BLUETOOTH:
		return "AF_BLUETOOTH"
	case AF_IUCV:
		return "AF_IUCV"
	case AF_RXRPC:
		return "AF_RXRPC"
	case AF_ISDN:
		return "AF_ISDN"
	case AF_PHONET:
		return "AF_PHONET"
	case AF_IEEE802154:
		return "AF_IEEE802154"
	case AF_CAIF:
		return "AF_CAIF"
	case AF_ALG:
		return "AF_ALG"
	case AF_NFC:
		return "AF_NFC"
	case AF_VSOCK:
		return "AF_VSOCK"
	case AF_KCM:
		return "AF_KCM"
	case AF_QIPCRTR:
		return "AF_QIPCRTR"
	case AF_SMC:
		return "AF_SMC"
	case AF_XDP:
		return "AF_XDP"
	case AF_MAX:
		return "AF_MAX"
	default:
		return fmt.Sprintf("SocketFamily(%v)", se)
	}
}

// SocketType - Type of a socket
type SocketType int32

const (
	SOCK_STREAM    SocketType = 1
	SOCK_DGRAM     SocketType = 2
	SOCK_RAW       SocketType = 3
	SOCK_RDM       SocketType = 4
	SOCK_SEQPACKET SocketType = 5
	SOCK_DCCP      SocketType = 6
	SOCK_PACKET    SocketType = 10
)

func SocketTypeToString(input int32) string {
	st := SocketType(input)
	switch st {
	case SOCK_DGRAM:
		return "SOCK_DGRAM"
	case SOCK_STREAM:
		return "SOCK_STREAM"
	case SOCK_RAW:
		return "SOCK_RAW"
	case SOCK_RDM:
		return "SOCK_RDM"
	case SOCK_SEQPACKET:
		return "SOCK_SEQPACKET"
	case SOCK_DCCP:
		return "SOCK_DCCP"
	case SOCK_PACKET:
		return "SOCK_PACKET"
	default:
		return fmt.Sprintf("SocketType(%v)", st)
	}
}

// SocketAction - Socket action type
type SocketAction string

const (
	// Create - A socket has just been created
	Create SocketAction = "create"
	// Connect - A socket is connecting to a remote addr
	Connect SocketAction = "connect"
	// Accept - A socket is accepting connection from a remote peer
	Accept SocketAction = "accept"
	// Bind - A socket is bound to a local IP
	Bind SocketAction = "bind"
)

// ErrValue - Return value
type ErrValue int32

const (
	EPERM                 ErrValue = 1      /* Operation not permitted */
	ENOENT                ErrValue = 2      /* No such file or directory */
	ESRCH                 ErrValue = 3      /* No such process */
	EINTR                 ErrValue = 4      /* Interrupted system call */
	EIO                   ErrValue = 5      /* I/O error */
	ENXIO                 ErrValue = 6      /* No such device or address */
	E2BIG                 ErrValue = 7      /* Argument list too long */
	ENOEXEC               ErrValue = 8      /* Exec format error */
	EBADF                 ErrValue = 9      /* Bad file number */
	ECHILD                ErrValue = 10     /* No child processes */
	EAGAIN                ErrValue = 11     /* Try again */
	ENOMEM                ErrValue = 12     /* Out of memory */
	EACCES                ErrValue = 13     /* Permission denied */
	EFAULT                ErrValue = 14     /* Bad address */
	ENOTBLK               ErrValue = 15     /* Block device required */
	EBUSY                 ErrValue = 16     /* Device or resource busy */
	EEXIST                ErrValue = 17     /* File exists */
	EXDEV                 ErrValue = 18     /* Cross-device link */
	ENODEV                ErrValue = 19     /* No such device */
	ENOTDIR               ErrValue = 20     /* Not a directory */
	EISDIR                ErrValue = 21     /* Is a directory */
	EINVAL                ErrValue = 22     /* Invalid argument */
	ENFILE                ErrValue = 23     /* File table overflow */
	EMFILE                ErrValue = 24     /* Too many open files */
	ENOTTY                ErrValue = 25     /* Not a typewriter */
	ETXTBSY               ErrValue = 26     /* Text file busy */
	EFBIG                 ErrValue = 27     /* File too large */
	ENOSPC                ErrValue = 28     /* No space left on device */
	ESPIPE                ErrValue = 29     /* Illegal seek */
	EROFS                 ErrValue = 30     /* Read-only file system */
	EMLINK                ErrValue = 31     /* Too many links */
	EPIPE                 ErrValue = 32     /* Broken pipe */
	EDOM                  ErrValue = 33     /* Math argument out of domain of func */
	ERANGE                ErrValue = 34     /* Math result not representable */
	EDEADLK               ErrValue = 35     /* Resource deadlock would occur */
	ENAMETOOLONG          ErrValue = 36     /* File name too long */
	ENOLCK                ErrValue = 37     /* No record locks available */
	ENOSYS                ErrValue = 38     /* Invalid system call number */
	ENOTEMPTY             ErrValue = 39     /* Directory not empty */
	ELOOP                 ErrValue = 40     /* Too many symbolic links encountered */
	EWOULDBLOCK           ErrValue = EAGAIN /* Operation would block */
	ENOMSG                ErrValue = 42     /* No message of desired type */
	EIDRM                 ErrValue = 43     /* Identifier removed */
	ECHRNG                ErrValue = 44     /* Channel number out of range */
	EL2NSYNC              ErrValue = 45     /* Level 2 not synchronized */
	EL3HLT                ErrValue = 46     /* Level 3 halted */
	EL3RST                ErrValue = 47     /* Level 3 reset */
	ELNRNG                ErrValue = 48     /* Link number out of range */
	EUNATCH               ErrValue = 49     /* Protocol driver not attached */
	ENOCSI                ErrValue = 50     /* No CSI structure available */
	EL2HLT                ErrValue = 51     /* Level 2 halted */
	EBADE                 ErrValue = 52     /* Invalid exchange */
	EBADR                 ErrValue = 53     /* Invalid request descriptor */
	EXFULL                ErrValue = 54     /* Exchange full */
	ENOANO                ErrValue = 55     /* No anode */
	EBADRQC               ErrValue = 56     /* Invalid request code */
	EBADSLT               ErrValue = 57     /* Invalid slot */
	EDEADLOCK             ErrValue = EDEADLK
	EBFONT                ErrValue = 59  /* Bad font file format */
	ENOSTR                ErrValue = 60  /* Device not a stream */
	ENODATA               ErrValue = 61  /* No data available */
	ETIME                 ErrValue = 62  /* Timer expired */
	ENOSR                 ErrValue = 63  /* Out of streams resources */
	ENONET                ErrValue = 64  /* Machine is not on the network */
	ENOPKG                ErrValue = 65  /* Package not installed */
	EREMOTE               ErrValue = 66  /* Object is remote */
	ENOLINK               ErrValue = 67  /* Link has been severed */
	EADV                  ErrValue = 68  /* Advertise error */
	ESRMNT                ErrValue = 69  /* Srmount error */
	ECOMM                 ErrValue = 70  /* Communication error on send */
	EPROTO                ErrValue = 71  /* Protocol error */
	EMULTIHOP             ErrValue = 72  /* Multihop attempted */
	EDOTDOT               ErrValue = 73  /* RFS specific error */
	EBADMSG               ErrValue = 74  /* Not a data message */
	EOVERFLOW             ErrValue = 75  /* Value too large for defined data type */
	ENOTUNIQ              ErrValue = 76  /* Name not unique on network */
	EBADFD                ErrValue = 77  /* File descriptor in bad state */
	EREMCHG               ErrValue = 78  /* Remote address changed */
	ELIBACC               ErrValue = 79  /* Can not access a needed shared library */
	ELIBBAD               ErrValue = 80  /* Accessing a corrupted shared library */
	ELIBSCN               ErrValue = 81  /* .lib section in a.out corrupted */
	ELIBMAX               ErrValue = 82  /* Attempting to link in too many shared libraries */
	ELIBEXEC              ErrValue = 83  /* Cannot exec a shared library directly */
	EILSEQ                ErrValue = 84  /* Illegal byte sequence */
	ERESTART              ErrValue = 85  /* Interrupted system call should be restarted */
	ESTRPIPE              ErrValue = 86  /* Streams pipe error */
	EUSERS                ErrValue = 87  /* Too many users */
	ENOTSOCK              ErrValue = 88  /* Socket operation on non-socket */
	EDESTADDRREQ          ErrValue = 89  /* Destination address required */
	EMSGSIZE              ErrValue = 90  /* Message too long */
	EPROTOTYPE            ErrValue = 91  /* Protocol wrong type for socket */
	ENOPROTOOPT           ErrValue = 92  /* Protocol not available */
	EPROTONOSUPPORT       ErrValue = 93  /* Protocol not supported */
	ESOCKTNOSUPPORT       ErrValue = 94  /* Socket type not supported */
	EOPNOTSUPP            ErrValue = 95  /* Operation not supported on transport endpoint */
	EPFNOSUPPORT          ErrValue = 96  /* Protocol family not supported */
	EAFNOSUPPORT          ErrValue = 97  /* Address family not supported by protocol */
	EADDRINUSE            ErrValue = 98  /* Address already in use */
	EADDRNOTAVAIL         ErrValue = 99  /* Cannot assign requested address */
	ENETDOWN              ErrValue = 100 /* Network is down */
	ENETUNREACH           ErrValue = 101 /* Network is unreachable */
	ENETRESET             ErrValue = 102 /* Network dropped connection because of reset */
	ECONNABORTED          ErrValue = 103 /* Software caused connection abort */
	ECONNRESET            ErrValue = 104 /* Connection reset by peer */
	ENOBUFS               ErrValue = 105 /* No buffer space available */
	EISCONN               ErrValue = 106 /* Transport endpoint is already connected */
	ENOTCONN              ErrValue = 107 /* Transport endpoint is not connected */
	ESHUTDOWN             ErrValue = 108 /* Cannot send after transport endpoint shutdown */
	ETOOMANYREFS          ErrValue = 109 /* Too many references: cannot splice */
	ETIMEDOUT             ErrValue = 110 /* Connection timed out */
	ECONNREFUSED          ErrValue = 111 /* Connection refused */
	EHOSTDOWN             ErrValue = 112 /* Host is down */
	EHOSTUNREACH          ErrValue = 113 /* No route to host */
	EALREADY              ErrValue = 114 /* Operation already in progress */
	EINPROGRESS           ErrValue = 115 /* Operation now in progress */
	ESTALE                ErrValue = 116 /* Stale file handle */
	EUCLEAN               ErrValue = 117 /* Structure needs cleaning */
	ENOTNAM               ErrValue = 118 /* Not a XENIX named type file */
	ENAVAIL               ErrValue = 119 /* No XENIX semaphores available */
	EISNAM                ErrValue = 120 /* Is a named type file */
	EREMOTEIO             ErrValue = 121 /* Remote I/O error */
	EDQUOT                ErrValue = 122 /* Quota exceeded */
	ENOMEDIUM             ErrValue = 123 /* No medium found */
	EMEDIUMTYPE           ErrValue = 124 /* Wrong medium type */
	ECANCELED             ErrValue = 125 /* Operation Canceled */
	ENOKEY                ErrValue = 126 /* Required key not available */
	EKEYEXPIRED           ErrValue = 127 /* Key has expired */
	EKEYREVOKED           ErrValue = 128 /* Key has been revoked */
	EKEYREJECTED          ErrValue = 129 /* Key was rejected by service */
	EOWNERDEAD            ErrValue = 130 /* Owner died */
	ENOTRECOVERABLE       ErrValue = 131 /* State not recoverable */
	ERFKILL               ErrValue = 132 /* Operation not possible due to RF-kill */
	EHWPOISON             ErrValue = 133 /* Memory page has hardware error */
	ERESTARTSYS           ErrValue = 512
	ERESTARTNOINTR        ErrValue = 513
	ERESTARTNOHAND        ErrValue = 514 /* restart if no handler.. */
	ENOIOCTLCMD           ErrValue = 515 /* No ioctl command */
	ERESTART_RESTARTBLOCK ErrValue = 516 /* restart by calling sys_restart_syscall */
	EPROBE_DEFER          ErrValue = 517 /* Driver requests probe retry */
	EOPENSTALE            ErrValue = 518 /* open found a stale dentry */
	ENOPARAM              ErrValue = 519 /* Parameter not supported */

	/* Defined for the NFSv3 protocol */
	EBADHANDLE      ErrValue = 521 /* Illegal NFS file handle */
	ENOTSYNC        ErrValue = 522 /* Update synchronization mismatch */
	EBADCOOKIE      ErrValue = 523 /* Cookie is stale */
	ENOTSUPP        ErrValue = 524 /* Operation is not supported */
	ETOOSMALL       ErrValue = 525 /* Buffer or request is too small */
	ESERVERFAULT    ErrValue = 526 /* An untranslatable error occurred */
	EBADTYPE        ErrValue = 527 /* Type not supported by server */
	EJUKEBOX        ErrValue = 528 /* Request initiated, but will not complete before timeout */
	EIOCBQUEUED     ErrValue = 529 /* iocb queued, will get completion event */
	ERECALLCONFLICT ErrValue = 530 /* conflict with recalled state */
)

// ErrValueToString - Returns an err as its string representation
func ErrValueToString(input int32) string {
	if input >= 0 {
		return fmt.Sprintf("%v", input)
	}
	switch ErrValue(-input) {
	case EPERM:
		return "EPERM"
	case ENOENT:
		return "ENOENT"
	case ESRCH:
		return "ESRCH"
	case EINTR:
		return "EINTR"
	case EIO:
		return "EIO"
	case ENXIO:
		return "ENXIO"
	case E2BIG:
		return "E2BIG"
	case ENOEXEC:
		return "ENOEXEC"
	case EBADF:
		return "EBADF"
	case ECHILD:
		return "ECHILD"
	case EAGAIN:
		return "EAGAIN"
	case ENOMEM:
		return "ENOMEM"
	case EACCES:
		return "EACCES"
	case EFAULT:
		return "EFAULT"
	case ENOTBLK:
		return "ENOTBLK"
	case EBUSY:
		return "EBUSY"
	case EEXIST:
		return "EEXIST"
	case EXDEV:
		return "EXDEV"
	case ENODEV:
		return "ENODEV"
	case ENOTDIR:
		return "ENOTDIR"
	case EISDIR:
		return "EISDIR"
	case EINVAL:
		return "EINVAL"
	case ENFILE:
		return "ENFILE"
	case EMFILE:
		return "EMFILE"
	case ENOTTY:
		return "ENOTTY"
	case ETXTBSY:
		return "ETXTBSY"
	case EFBIG:
		return "EFBIG"
	case ENOSPC:
		return "ENOSPC"
	case ESPIPE:
		return "ESPIPE"
	case EROFS:
		return "EROFS"
	case EMLINK:
		return "EMLINK"
	case EPIPE:
		return "EPIPE"
	case EDOM:
		return "EDOM"
	case ERANGE:
		return "ERANGE"
	case EDEADLK:
		return "EDEADLK"
	case ENAMETOOLONG:
		return "ENAMETOOLONG"
	case ENOLCK:
		return "ENOLCK"
	case ENOSYS:
		return "ENOSYS"
	case ENOTEMPTY:
		return "ENOTEMPTY"
	case ELOOP:
		return "ELOOP"
	case ENOMSG:
		return "ENOMSG"
	case EIDRM:
		return "EIDRM"
	case ECHRNG:
		return "ECHRNG"
	case EL2NSYNC:
		return "EL2NSYNC"
	case EL3HLT:
		return "EL3HLT"
	case EL3RST:
		return "EL3RST"
	case ELNRNG:
		return "ELNRNG"
	case EUNATCH:
		return "EUNATCH"
	case ENOCSI:
		return "ENOCSI"
	case EL2HLT:
		return "EL2HLT"
	case EBADE:
		return "EBADE"
	case EBADR:
		return "EBADR"
	case EXFULL:
		return "EXFULL"
	case ENOANO:
		return "ENOANO"
	case EBADRQC:
		return "EBADRQC"
	case EBADSLT:
		return "EBADSLT"
	case EBFONT:
		return "EBFONT"
	case ENOSTR:
		return "ENOSTR"
	case ENODATA:
		return "ENODATA"
	case ETIME:
		return "ETIME"
	case ENOSR:
		return "ENOSR"
	case ENONET:
		return "ENONET"
	case ENOPKG:
		return "ENOPKG"
	case EREMOTE:
		return "EREMOTE"
	case ENOLINK:
		return "ENOLINK"
	case EADV:
		return "EADV"
	case ESRMNT:
		return "ESRMNT"
	case ECOMM:
		return "ECOMM"
	case EPROTO:
		return "EPROTO"
	case EMULTIHOP:
		return "EMULTIHOP"
	case EDOTDOT:
		return "EDOTDOT"
	case EBADMSG:
		return "EBADMSG"
	case EOVERFLOW:
		return "EOVERFLOW"
	case ENOTUNIQ:
		return "ENOTUNIQ"
	case EBADFD:
		return "EBADFD"
	case EREMCHG:
		return "EREMCHG"
	case ELIBACC:
		return "ELIBACC"
	case ELIBBAD:
		return "ELIBBAD"
	case ELIBSCN:
		return "ELIBSCN"
	case ELIBMAX:
		return "ELIBMAX"
	case ELIBEXEC:
		return "ELIBEXEC"
	case EILSEQ:
		return "EILSEQ"
	case ERESTART:
		return "ERESTART"
	case ESTRPIPE:
		return "ESTRPIPE"
	case EUSERS:
		return "EUSERS"
	case ENOTSOCK:
		return "ENOTSOCK"
	case EDESTADDRREQ:
		return "EDESTADDRREQ"
	case EMSGSIZE:
		return "EMSGSIZE"
	case EPROTOTYPE:
		return "EPROTOTYPE"
	case ENOPROTOOPT:
		return "ENOPROTOOPT"
	case EPROTONOSUPPORT:
		return "EPROTONOSUPPORT"
	case ESOCKTNOSUPPORT:
		return "ESOCKTNOSUPPORT"
	case EOPNOTSUPP:
		return "EOPNOTSUPP"
	case EPFNOSUPPORT:
		return "EPFNOSUPPORT"
	case EAFNOSUPPORT:
		return "EAFNOSUPPORT"
	case EADDRINUSE:
		return "EADDRINUSE"
	case EADDRNOTAVAIL:
		return "EADDRNOTAVAIL"
	case ENETDOWN:
		return "ENETDOWN"
	case ENETUNREACH:
		return "ENETUNREACH"
	case ENETRESET:
		return "ENETRESET"
	case ECONNABORTED:
		return "ECONNABORTED"
	case ECONNRESET:
		return "ECONNRESET"
	case ENOBUFS:
		return "ENOBUFS"
	case EISCONN:
		return "EISCONN"
	case ENOTCONN:
		return "ENOTCONN"
	case ESHUTDOWN:
		return "ESHUTDOWN"
	case ETOOMANYREFS:
		return "ETOOMANYREFS"
	case ETIMEDOUT:
		return "ETIMEDOUT"
	case ECONNREFUSED:
		return "ECONNREFUSED"
	case EHOSTDOWN:
		return "EHOSTDOWN"
	case EHOSTUNREACH:
		return "EHOSTUNREACH"
	case EALREADY:
		return "EALREADY"
	case EINPROGRESS:
		return "EINPROGRESS"
	case ESTALE:
		return "ESTALE"
	case EUCLEAN:
		return "EUCLEAN"
	case ENOTNAM:
		return "ENOTNAM"
	case ENAVAIL:
		return "ENAVAIL"
	case EISNAM:
		return "EISNAM"
	case EREMOTEIO:
		return "EREMOTEIO"
	case EDQUOT:
		return "EDQUOT"
	case ENOMEDIUM:
		return "ENOMEDIUM"
	case EMEDIUMTYPE:
		return "EMEDIUMTYPE"
	case ECANCELED:
		return "ECANCELED"
	case ENOKEY:
		return "ENOKEY"
	case EKEYEXPIRED:
		return "EKEYEXPIRED"
	case EKEYREVOKED:
		return "EKEYREVOKED"
	case EKEYREJECTED:
		return "EKEYREJECTED"
	case EOWNERDEAD:
		return "EOWNERDEAD"
	case ENOTRECOVERABLE:
		return "ENOTRECOVERABLE"
	case ERFKILL:
		return "ERFKILL"
	case EHWPOISON:
		return "EHWPOISON"
	case ERESTARTSYS:
		return "ERESTARTSYS"
	case ERESTARTNOINTR:
		return "ERESTARTNOINTR"
	case ERESTARTNOHAND:
		return "ERESTARTNOHAND"
	case ENOIOCTLCMD:
		return "ENOIOCTLCMD"
	case ERESTART_RESTARTBLOCK:
		return "ERESTART_RESTARTBLOCK"
	case EPROBE_DEFER:
		return "EPROBE_DEFER"
	case EOPENSTALE:
		return "EOPENSTALE"
	case ENOPARAM:
		return "ENOPARAM"
	case EBADHANDLE:
		return "EBADHANDLE"
	case ENOTSYNC:
		return "ENOTSYNC"
	case EBADCOOKIE:
		return "EBADCOOKIE"
	case ENOTSUPP:
		return "ENOTSUPP"
	case ETOOSMALL:
		return "ETOOSMALL"
	case ESERVERFAULT:
		return "ESERVERFAULT"
	case EBADTYPE:
		return "EBADTYPE"
	case EJUKEBOX:
		return "EJUKEBOX"
	case EIOCBQUEUED:
		return "EIOCBQUEUED"
	case ERECALLCONFLICT:
		return "ERECALLCONFLICT"
	default:
		return fmt.Sprintf("Err(%v)", input)
	}
}

// OpenFlag - Open syscall flag
type OpenFlag int

const (
	O_ACCMODE   OpenFlag = 3
	O_RDONLY    OpenFlag = 0
	O_WRONLY    OpenFlag = 1
	O_RDWR      OpenFlag = 2
	O_CREAT     OpenFlag = 64
	O_EXCL      OpenFlag = 128
	O_NOCTTY    OpenFlag = 256
	O_TRUNC     OpenFlag = 512
	O_APPEND    OpenFlag = 1024
	O_NONBLOCK  OpenFlag = 2048
	O_DSYNC     OpenFlag = 4096  /* used to be O_SYNC, see below */
	FASYNC      OpenFlag = 8192  /* fcntl, for BSD compatibility */
	O_DIRECT    OpenFlag = 16384 /* direct disk access hint */
	O_LARGEFILE OpenFlag = 32768
	O_DIRECTORY OpenFlag = 65536  /* must be a directory */
	O_NOFOLLOW  OpenFlag = 131072 /* don't follow links */
	O_NOATIME   OpenFlag = 262144
	O_CLOEXEC   OpenFlag = 524288 /* set close_on_exec */
)

// OpenFlagToString - Returns the string representation of an open flag
func OpenFlagToString(of OpenFlag) string {
	switch of {
	case O_ACCMODE:
		return "O_ACCMODE"
	case O_RDONLY:
		return "O_RDONLY"
	case O_WRONLY:
		return "O_WRONLY"
	case O_RDWR:
		return "O_RDWR"
	case O_CREAT:
		return "O_CREAT"
	case O_EXCL:
		return "O_EXCL"
	case O_NOCTTY:
		return "O_NOCTTY"
	case O_TRUNC:
		return "O_TRUNC"
	case O_APPEND:
		return "O_APPEND"
	case O_NONBLOCK:
		return "O_NONBLOCK"
	case O_DSYNC:
		return "O_DSYNC"
	case FASYNC:
		return "FASYNC"
	case O_DIRECT:
		return "O_DIRECT"
	case O_LARGEFILE:
		return "O_LARGEFILE"
	case O_DIRECTORY:
		return "O_DIRECTORY"
	case O_NOFOLLOW:
		return "O_NOFOLLOW"
	case O_NOATIME:
		return "O_NOATIME"
	case O_CLOEXEC:
		return "O_CLOEXEC"
	default:
		return fmt.Sprintf("OpenFlag(%#o)", of)
	}
}

// OpenFlagsToStrings - Returns the string list version of flags
func OpenFlagsToStrings(input int32) []string {
	flags := OpenFlag(input)
	rep := []string{}
	if flags&O_ACCMODE == O_ACCMODE {
		rep = append(rep, OpenFlagToString(O_ACCMODE))
	}
	if flags&O_RDONLY == O_RDONLY {
		rep = append(rep, OpenFlagToString(O_RDONLY))
	}
	if flags&O_WRONLY == O_WRONLY {
		rep = append(rep, OpenFlagToString(O_WRONLY))
	}
	if flags&O_RDWR == O_RDWR {
		rep = append(rep, OpenFlagToString(O_RDWR))
	}
	if flags&O_CREAT == O_CREAT {
		rep = append(rep, OpenFlagToString(O_CREAT))
	}
	if flags&O_EXCL == O_EXCL {
		rep = append(rep, OpenFlagToString(O_EXCL))
	}
	if flags&O_NOCTTY == O_NOCTTY {
		rep = append(rep, OpenFlagToString(O_NOCTTY))
	}
	if flags&O_TRUNC == O_TRUNC {
		rep = append(rep, OpenFlagToString(O_TRUNC))
	}
	if flags&O_APPEND == O_APPEND {
		rep = append(rep, OpenFlagToString(O_APPEND))
	}
	if flags&O_NONBLOCK == O_NONBLOCK {
		rep = append(rep, OpenFlagToString(O_NONBLOCK))
	}
	if flags&O_DSYNC == O_DSYNC {
		rep = append(rep, OpenFlagToString(O_DSYNC))
	}
	if flags&FASYNC == FASYNC {
		rep = append(rep, OpenFlagToString(FASYNC))
	}
	if flags&O_DIRECT == O_DIRECT {
		rep = append(rep, OpenFlagToString(O_DIRECT))
	}
	if flags&O_LARGEFILE == O_LARGEFILE {
		rep = append(rep, OpenFlagToString(O_LARGEFILE))
	}
	if flags&O_DIRECTORY == O_DIRECTORY {
		rep = append(rep, OpenFlagToString(O_DIRECTORY))
	}
	if flags&O_NOFOLLOW == O_NOFOLLOW {
		rep = append(rep, OpenFlagToString(O_NOFOLLOW))
	}
	if flags&O_NOATIME == O_NOATIME {
		rep = append(rep, OpenFlagToString(O_NOATIME))
	}
	if flags&O_CLOEXEC == O_CLOEXEC {
		rep = append(rep, OpenFlagToString(O_CLOEXEC))
	}
	return rep
}

type AccMode int32

const (
	MAY_EXEC   AccMode = 0x00000001
	MAY_WRITE  AccMode = 0x00000002
	MAY_READ   AccMode = 0x00000004
	MAY_APPEND AccMode = 0x00000008
	MAY_ACCESS AccMode = 0x00000010
	MAY_OPEN   AccMode = 0x00000020
	MAY_CHDIR  AccMode = 0x00000040
)

// AccModeToString - Returns the string representation of an acc_mode
func AccModeToString(am AccMode) string {
	switch am {
	case MAY_EXEC:
		return "MAY_EXEC"
	case MAY_WRITE:
		return "MAY_WRITE"
	case MAY_READ:
		return "MAY_READ"
	case MAY_APPEND:
		return "MAY_APPEND"
	case MAY_ACCESS:
		return "MAY_ACCESS"
	case MAY_OPEN:
		return "MAY_OPEN"
	case MAY_CHDIR:
		return "MAY_CHDIR"
	default:
		return fmt.Sprintf("AccMode(%v)", am)
	}
}

// AccModesToStrings - Returns the string list version of acc_mode
func AccModesToStrings(input int32) []string {
	am := AccMode(input)
	rep := []string{}
	if am&MAY_EXEC == MAY_EXEC {
		rep = append(rep, AccModeToString(MAY_EXEC))
	}
	if am&MAY_WRITE == MAY_WRITE {
		rep = append(rep, AccModeToString(MAY_WRITE))
	}
	if am&MAY_READ == MAY_READ {
		rep = append(rep, AccModeToString(MAY_READ))
	}
	if am&MAY_APPEND == MAY_APPEND {
		rep = append(rep, AccModeToString(MAY_APPEND))
	}
	if am&MAY_ACCESS == MAY_ACCESS {
		rep = append(rep, AccModeToString(MAY_ACCESS))
	}
	if am&MAY_OPEN == MAY_OPEN {
		rep = append(rep, AccModeToString(MAY_OPEN))
	}
	if am&MAY_CHDIR == MAY_CHDIR {
		rep = append(rep, AccModeToString(MAY_CHDIR))
	}
	return rep
}

// ExecveFlag - Execve flag
type ExecveFlag int32

const (
	// FileNotFound - Indicates that the file doesn't exist
	FileNotFound ExecveFlag = 0
	// FilenameSet - Indicates that a filename is set
	FilenameSet ExecveFlag = 1 << 0
	// DynamicallyLinked - Indicates that the elf file is dynamically linked
	DynamicallyLinked ExecveFlag = 1 << 1
)

// ExecveFlagToStrings - Retuns the string list version of execve flags
func ExecveFlagToStrings(input int32) []string {
	if input == 0 {
		return []string{"FileNotFound"}
	}
	ef := ExecveFlag(input)
	rep := []string{}
	if ef&FilenameSet == FilenameSet {
		rep = append(rep, "FilenameSet")
	}
	if ef&DynamicallyLinked == DynamicallyLinked {
		rep = append(rep, "DynamicallyLinked")
	}
	return rep
}

// SecurebitsFlag - Securebits flag
type SecurebitsFlag uint32

const (
	// SecurebitsDefault - Default value
	SecurebitsDefault SecurebitsFlag = 0
	// SecbitNoRoot - When set UID 0 has no special privileges. When unset,
	// we support inheritance of root-permissions and suid-root executable
	// under compatibility mode. We raise the effective and inheritable bitmasks
	// *of the executable file* if the effective uid of the new process is
	// 0. If the real uid is 0, we raise the effective (legacy) bit of the
	// executable file.
	SecbitNoRoot SecurebitsFlag = 1 << 0
	// SecbitNoRootLocked - A setting which is locked cannot be changed from user-level.
	SecbitNoRootLocked SecurebitsFlag = 1 << 1
	// SecbitNoSetuidFixup - When set, setuid to/from uid 0 does not trigger
	// capability-"fixup". When unset, to provide compatiblility with old
	// programs relying on set*uid to gain/lose privilege, transitions
	// to/from uid 0 cause capabilities to be gained/lost.
	SecbitNoSetuidFixup SecurebitsFlag = 1 << 2
	// SecbitNoSetuidFixupLocked - A setting which is locked cannot be changed from user-level.
	SecbitNoSetuidFixupLocked SecurebitsFlag = 1 << 3
	// SecbitKeepCaps - When set, a process can retain its capabilities
	// even after transitioning to a non-root user (the set-uid fixup
	// suppressed by bit 2). Bit-4 is cleared when a process calls exec();
	// setting both bit 4 and 5 will create a barrier through exec that
	// no exec()'d child can use this feature again.
	SecbitKeepCaps SecurebitsFlag = 1 << 4
	// SecbitKeepCapsLocked - A setting which is locked cannot be changed from user-level.
	SecbitKeepCapsLocked SecurebitsFlag = 1 << 5
	// SecbitNoCapAmbienRaise - When set, a process cannot add new capabilities to its ambient set.
	SecbitNoCapAmbienRaise SecurebitsFlag = 1 << 6
	// SecbitNoCapAmbienRaiseLocked - A setting which is locked cannot be changed from user-level.
	SecbitNoCapAmbienRaiseLocked SecurebitsFlag = 1 << 7
	// SecureAllBits - Activates all secure bits
	SecureAllBits SecurebitsFlag = 0x55
	// SecureAllLocks - Activates all locks
	SecureAllLocks SecurebitsFlag = SecureAllBits << 1
	// SecureAllBitsAndLocks - Activates all secure bits and locks
	SecureAllBitsAndLocks SecurebitsFlag = SecureAllBits | SecureAllLocks
)

// SecurebitsFlagToStrings - Returns a string list representation of a securebits flag
func SecurebitsFlagToStrings(flag uint32) []string {
	sflag := SecurebitsFlag(flag)
	switch sflag {
	case SecurebitsDefault:
		return []string{"SecurebitsDefault"}
	case SecureAllBitsAndLocks:
		return []string{"SecureAllBitsAndLocks"}
	case SecureAllBits:
		return []string{"SecureAllBits"}
	case SecureAllLocks:
		return []string{"SecureAllLocks"}
	}
	rep := []string{}
	if sflag&SecbitNoRoot == SecbitNoRoot {
		rep = append(rep, "SecbitNoRoot")
	}
	if sflag&SecbitNoRootLocked == SecbitNoRootLocked {
		rep = append(rep, "SecbitNoRootLocked")
	}
	if sflag&SecbitNoSetuidFixup == SecbitNoSetuidFixup {
		rep = append(rep, "SecbitNoSetuidFixup")
	}
	if sflag&SecbitNoSetuidFixupLocked == SecbitNoSetuidFixupLocked {
		rep = append(rep, "SecbitNoSetuidFixupLocked")
	}
	if sflag&SecbitKeepCaps == SecbitKeepCaps {
		rep = append(rep, "SecbitKeepCaps")
	}
	if sflag&SecbitKeepCapsLocked == SecbitKeepCapsLocked {
		rep = append(rep, "SecbitKeepCapsLocked")
	}
	if sflag&SecbitNoCapAmbienRaise == SecbitNoCapAmbienRaise {
		rep = append(rep, "SecbitNoCapAmbienRaise")
	}
	if sflag&SecbitNoCapAmbienRaiseLocked == SecbitNoCapAmbienRaiseLocked {
		rep = append(rep, "SecbitNoCapAmbienRaiseLocked")
	}
	return rep
}

// ContainerAction - Container action enum
type ContainerAction string

const (
	ContainerUnknown    ContainerAction = "UNKNOWN"
	ContainerCreated    ContainerAction = "CREATED"
	ContainerRunning    ContainerAction = "RUNNING"
	ContainerExited     ContainerAction = "EXITED"
	ContainerDestroyed  ContainerAction = "DESTROYED"
	ContainerExec       ContainerAction = "EXEC"
	ContainerAttach     ContainerAction = "ATTACH"
	ContainerConnect    ContainerAction = "CONNECT"
	ContainerDisconnect ContainerAction = "DISCONNECT"
)

// Syscalls
var syscalls = map[uint32]string{
	0:   "read",
	1:   "write",
	2:   "open",
	3:   "close",
	4:   "stat",
	5:   "fstat",
	6:   "lstat",
	7:   "poll",
	8:   "lseek",
	9:   "mmap",
	10:  "mprotect",
	11:  "munmap",
	12:  "brk",
	13:  "rt_sigaction",
	14:  "rt_sigprocmask",
	15:  "rt_sigreturn",
	16:  "ioctl",
	17:  "pread64",
	18:  "pwrite64",
	19:  "readv",
	20:  "writev",
	21:  "access",
	22:  "pipe",
	23:  "select",
	24:  "sched_yield",
	25:  "mremap",
	26:  "msync",
	27:  "mincore",
	28:  "madvise",
	29:  "shmget",
	30:  "shmat",
	31:  "shmctl",
	32:  "dup",
	33:  "dup2",
	34:  "pause",
	35:  "nanosleep",
	36:  "getitimer",
	37:  "alarm",
	38:  "setitimer",
	39:  "getpid",
	40:  "sendfile",
	41:  "socket",
	42:  "connect",
	43:  "accept",
	44:  "sendto",
	45:  "recvfrom",
	46:  "sendmsg",
	47:  "recvmsg",
	48:  "shutdown",
	49:  "bind",
	50:  "listen",
	51:  "getsockname",
	52:  "getpeername",
	53:  "socketpair",
	54:  "setsockopt",
	55:  "getsockopt",
	56:  "clone",
	57:  "fork",
	58:  "vfork",
	59:  "execve",
	60:  "exit",
	61:  "wait4",
	62:  "kill",
	63:  "uname",
	64:  "semget",
	65:  "semop",
	66:  "semctl",
	67:  "shmdt",
	68:  "msgget",
	69:  "msgsnd",
	70:  "msgrcv",
	71:  "msgctl",
	72:  "fcntl",
	73:  "flock",
	74:  "fsync",
	75:  "fdatasync",
	76:  "truncate",
	77:  "ftruncate",
	78:  "getdents",
	79:  "getcwd",
	80:  "chdir",
	81:  "fchdir",
	82:  "rename",
	83:  "mkdir",
	84:  "rmdir",
	85:  "creat",
	86:  "link",
	87:  "unlink",
	88:  "symlink",
	89:  "readlink",
	90:  "chmod",
	91:  "fchmod",
	92:  "chown",
	93:  "fchown",
	94:  "lchown",
	95:  "umask",
	96:  "gettimeofday",
	97:  "getrlimit",
	98:  "getrusage",
	99:  "sysinfo",
	100: "times",
	101: "ptrace",
	102: "getuid",
	103: "syslog",
	104: "getgid",
	105: "setuid",
	106: "setgid",
	107: "geteuid",
	108: "getegid",
	109: "setpgid",
	110: "getppid",
	111: "getpgrp",
	112: "setsid",
	113: "setreuid",
	114: "setregid",
	115: "getgroups",
	116: "setgroups",
	117: "setresuid",
	118: "getresuid",
	119: "setresgid",
	120: "getresgid",
	121: "getpgid",
	122: "setfsuid",
	123: "setfsgid",
	124: "getsid",
	125: "capget",
	126: "capset",
	127: "rt_sigpending",
	128: "rt_sigtimedwait",
	129: "rt_sigqueueinfo",
	130: "rt_sigsuspend",
	131: "sigaltstack",
	132: "utime",
	133: "mknod",
	134: "uselib",
	135: "personality",
	136: "ustat",
	137: "statfs",
	138: "fstatfs",
	139: "sysfs",
	140: "getpriority",
	141: "setpriority",
	142: "sched_setparam",
	143: "sched_getparam",
	144: "sched_setscheduler",
	145: "sched_getscheduler",
	146: "sched_get_priority_max",
	147: "sched_get_priority_min",
	148: "sched_rr_get_interval",
	149: "mlock",
	150: "munlock",
	151: "mlockall",
	152: "munlockall",
	153: "vhangup",
	154: "modify_ldt",
	155: "pivot_root",
	156: "_sysctl",
	157: "prctl",
	158: "arch_prctl",
	159: "adjtimex",
	160: "setrlimit",
	161: "chroot",
	162: "sync",
	163: "acct",
	164: "settimeofday",
	165: "mount",
	166: "umount2",
	167: "swapon",
	168: "swapoff",
	169: "reboot",
	170: "sethostname",
	171: "setdomainname",
	172: "iopl",
	173: "ioperm",
	174: "create_module",
	175: "init_module",
	176: "delete_module",
	177: "get_kernel_syms",
	178: "query_module",
	179: "quotactl",
	180: "nfsservctl",
	181: "getpmsg",
	182: "putpmsg",
	183: "afs_syscall",
	184: "tuxcall",
	185: "security",
	186: "gettid",
	187: "readahead",
	188: "setxattr",
	189: "lsetxattr",
	190: "fsetxattr",
	191: "getxattr",
	192: "lgetxattr",
	193: "fgetxattr",
	194: "listxattr",
	195: "llistxattr",
	196: "flistxattr",
	197: "removexattr",
	198: "lremovexattr",
	199: "fremovexattr",
	200: "tkill",
	201: "time",
	202: "futex",
	203: "sched_setaffinity",
	204: "sched_getaffinity",
	205: "set_thread_area",
	206: "io_setup",
	207: "io_destroy",
	208: "io_getevents",
	209: "io_submit",
	210: "io_cancel",
	211: "get_thread_area",
	212: "lookup_dcookie",
	213: "epoll_create",
	214: "epoll_ctl_old",
	215: "epoll_wait_old",
	216: "remap_file_pages",
	217: "getdents64",
	218: "set_tid_address",
	219: "restart_syscall",
	220: "semtimedop",
	221: "fadvise64",
	222: "timer_create",
	223: "timer_settime",
	224: "timer_gettime",
	225: "timer_getoverrun",
	226: "timer_delete",
	227: "clock_settime",
	228: "clock_gettime",
	229: "clock_getres",
	230: "clock_nanosleep",
	231: "exit_group",
	232: "epoll_wait",
	233: "epoll_ctl",
	234: "tgkill",
	235: "utimes",
	236: "vserver",
	237: "mbind",
	238: "set_mempolicy",
	239: "get_mempolicy",
	240: "mq_open",
	241: "mq_unlink",
	242: "mq_timedsend",
	243: "mq_timedreceive",
	244: "mq_notify",
	245: "mq_getsetattr",
	246: "kexec_load",
	247: "waitid",
	248: "add_key",
	249: "request_key",
	250: "keyctl",
	251: "ioprio_set",
	252: "ioprio_get",
	253: "inotify_init",
	254: "inotify_add_watch",
	255: "inotify_rm_watch",
	256: "migrate_pages",
	257: "openat",
	258: "mkdirat",
	259: "mknodat",
	260: "fchownat",
	261: "futimesat",
	262: "newfstatat",
	263: "unlinkat",
	264: "renameat",
	265: "linkat",
	266: "symlinkat",
	267: "readlinkat",
	268: "fchmodat",
	269: "faccessat",
	270: "pselect6",
	271: "ppoll",
	272: "unshare",
	273: "set_robust_list",
	274: "get_robust_list",
	275: "splice",
	276: "tee",
	277: "sync_file_range",
	278: "vmsplice",
	279: "move_pages",
	280: "utimensat",
	281: "epoll_pwait",
	282: "signalfd",
	283: "timerfd_create",
	284: "eventfd",
	285: "fallocate",
	286: "timerfd_settime",
	287: "timerfd_gettime",
	288: "accept4",
	289: "signalfd4",
	290: "eventfd2",
	291: "epoll_create1",
	292: "dup3",
	293: "pipe2",
	294: "inotify_init1",
	295: "preadv",
	296: "pwritev",
	297: "rt_tgsigqueueinfo",
	298: "perf_event_open",
	299: "recvmmsg",
	300: "fanotify_init",
	301: "fanotify_mark",
	302: "prlimit64",
	303: "name_to_handle_at",
	304: "open_by_handle_at",
	305: "clock_adjtime",
	306: "syncfs",
	307: "sendmmsg",
	308: "setns",
	309: "getcpu",
	310: "process_vm_readv",
	311: "process_vm_writev",
	312: "kcmp",
	313: "finit_module",
	314: "sched_setattr",
	315: "sched_getattr",
	316: "renameat2",
	317: "seccomp",
	318: "getrandom",
	319: "memfd_create",
	320: "kexec_file_load",
	321: "bpf",
	322: "execveat",
	323: "userfaultfd",
	324: "membarrier",
	325: "mlock2",
	326: "copy_file_range",
	327: "preadv2",
	328: "pwritev2",
	329: "pkey_mprotect",
	330: "pkey_alloc",
	331: "pkey_free",
	332: "statx",
	333: "io_pgetevents",
	334: "rseq",
}

// GetSyscallName - Returns a syscall name from its id
func GetSyscallName(id uint32) string {
	name, ok := syscalls[id]
	if ok {
		return name
	}
	return fmt.Sprintf("[unknown num:%v]", id)
}

// KernelCapability - Kernel capability
type KernelCapability uint64

const (
	// CapChown :
	//   In a system with the [_POSIX_CHOWN_RESTRICTED] option defined, this
	//   overrides the restriction of changing file ownership and group
	//   ownership.
	CapChown KernelCapability = 1 << 0
	// CapDacOverride :
	//   Override all DAC access, including ACL execute access if
	//   [_POSIX_ACL] is defined. Excluding DAC access covered by
	//   CAP_LINUX_IMMUTABLE.
	CapDacOverride KernelCapability = 1 << 1
	// CapDacReadSearch :
	//   Overrides all DAC restrictions regarding read and search on files
	//   and directories, including ACL restrictions if [_POSIX_ACL] is
	//   defined. Excluding DAC access covered by CAP_LINUX_IMMUTABLE.
	CapDacReadSearch KernelCapability = 1 << 2
	// CapFowner :
	//   Overrides all restrictions about allowed operations on files, where
	//   file owner ID must be equal to the user ID, except where CAP_FSETID
	//   is applicable. It doesn't override MAC and DAC restrictions.
	CapFowner KernelCapability = 1 << 3
	// CapFsetid :
	//   Overrides the following restrictions that the effective user ID
	//   shall match the file owner ID when setting the S_ISUID and S_ISGID
	//   bits on that file; that the effective group ID (or one of the
	//   supplementary group IDs) shall match the file owner ID when setting
	//   the S_ISGID bit on that file; that the S_ISUID and S_ISGID bits are
	//   cleared on successful return from chown(2) (not implemented).
	CapFsetid KernelCapability = 1 << 4
	// CapKill :
	// Overrides the restriction that the real or effective user ID of a
	//   process sending a signal must match the real or effective user ID
	//   of the process receiving the signal.
	CapKill KernelCapability = 1 << 5
	// CapSetgid :
	//   Allows setgid(2) manipulation
	//   Allows setgroups(2)
	//   Allows forged gids on socket credentials passing.
	CapSetgid KernelCapability = 1 << 6
	// CapSetuid :
	//   Allows set*uid(2) manipulation (including fsuid).
	//   Allows forged pids on socket credentials passing.
	CapSetuid KernelCapability = 1 << 7
	// CapSetpcap :
	//   Without VFS support for capabilities:
	//     Transfer any capability in your permitted set to any pid,
	//     remove any capability in your permitted set from any pid
	//   With VFS support for capabilities (neither of above, but)
	//     Add any capability from current's capability bounding set
	//         to the current process' inheritable set
	//     Allow taking bits out of capability bounding set
	//     Allow modification of the securebits for a process
	//
	CapSetpcap KernelCapability = 1 << 8
	// CapLinuxImmutable :
	//   Allow modification of S_IMMUTABLE and S_APPEND file attributes
	CapLinuxImmutable KernelCapability = 1 << 9
	// CapNetBindService :
	//   Allows binding to TCP/UDP sockets below 1024
	//   Allows binding to ATM VCIs below 32
	CapNetBindService KernelCapability = 1 << 10
	// CapNetBroadcast :
	//   Allow broadcasting, listen to multicast
	CapNetBroadcast KernelCapability = 1 << 11
	// CapNetAdmin :
	//   Allow interface configuration
	//   Allow administration of IP firewall, masquerading and accounting
	//   Allow setting debug option on sockets
	//   Allow modification of routing tables
	//   Allow setting arbitrary process / process group ownership on
	//     sockets
	//   Allow binding to any address for transparent proxying (also via NET_RAW)
	//   Allow setting TOS (type of service)
	//   Allow setting promiscuous mode
	//   Allow clearing driver statistics
	//   Allow multicasting
	//   Allow read/write of device-specific registers
	//   Allow activation of ATM control sockets
	CapNetAdmin KernelCapability = 1 << 12
	// CapNetRaw :
	//   Allow use of RAW sockets
	//   Allow use of PACKET sockets
	//   Allow binding to any address for transparent proxying (also via NET_ADMIN)
	CapNetRaw KernelCapability = 1 << 13
	// CapIpcLock :
	//   Allow locking of shared memory segments
	//   Allow mlock and mlockall (which doesn't really have anything to do
	//   with IPC)
	CapIpcLock KernelCapability = 1 << 14
	// CapIpcOwner :
	//   Override IPC ownership checks
	CapIpcOwner KernelCapability = 1 << 15
	// CapSysModule :
	//   Insert and remove kernel modules - modify kernel without limit
	CapSysModule KernelCapability = 1 << 16
	// CapSysRawio :
	//   Allow ioperm/iopl access
	//   Allow sending USB messages to any device via /dev/bus/usb
	CapSysRawio KernelCapability = 1 << 17
	// CapSysChroot :
	//   Allow use of chroot()
	CapSysChroot KernelCapability = 1 << 18
	// CapSysPtrace :
	//   Allow ptrace() of any process
	CapSysPtrace KernelCapability = 1 << 19
	// CapSysPacct :
	//   Allow configuration of process accounting
	CapSysPacct KernelCapability = 1 << 20
	// CapSysAdmin :
	//   Allow configuration of the secure attention key
	//   Allow administration of the random device
	//   Allow examination and configuration of disk quotas
	//   Allow setting the domainname
	//   Allow setting the hostname
	//   Allow calling bdflush()
	//   Allow mount() and umount(), setting up new smb connection
	//   Allow some autofs root ioctls
	//   Allow nfsservctl
	//   Allow VM86_REQUEST_IRQ
	//   Allow to read/write pci config on alpha
	//   Allow irix_prctl on mips (setstacksize)
	//   Allow flushing all cache on m68k (sys_cacheflush)
	//   Allow removing semaphores
	//     Used instead of CAP_CHOWN to "chown" IPC message queues, semaphores
	//     and shared memory
	//   Allow locking/unlocking of shared memory segment
	//   Allow turning swap on/off
	//   Allow forged pids on socket credentials passing
	//   Allow setting readahead and flushing buffers on block devices
	//   Allow setting geometry in floppy driver
	//   Allow turning DMA on/off in xd driver
	//   Allow administration of md devices (mostly the above, but some
	//   extra ioctls)
	//   Allow tuning the ide driver
	//   Allow access to the nvram device
	//   Allow administration of apm_bios, serial and bttv (TV) device
	//   Allow manufacturer commands in isdn CAPI support driver
	//   Allow reading non-standardized portions of pci configuration space
	//   Allow DDI debug ioctl on sbpcd driver
	//   Allow setting up serial ports
	//   Allow sending raw qic-117 commands
	//   Allow enabling/disabling tagged queuing on SCSI controllers and sending
	//   arbitrary SCSI commands
	//   Allow setting encryption key on loopback filesystem
	//   Allow setting zone reclaim policy
	CapSysAdmin KernelCapability = 1 << 21
	// CapSysBoot :
	//   Allow use of reboot()
	CapSysBoot KernelCapability = 1 << 22
	// CapSysNice :
	//   Allow raising priority and setting priority on other (different
	//   UID) processes
	//   Allow use of FIFO and round-robin (realtime) scheduling on own
	//   processes and setting the scheduling algorithm used by another
	//   process.
	//   Allow setting cpu affinity on other processes
	CapSysNice KernelCapability = 1 << 23
	// CapSysResource :
	// Override resource limits. Set resource limits.
	// Override quota limits.
	// Override reserved space on ext2 filesystem
	// Modify data journaling mode on ext3 filesystem (uses journaling
	//   resources)
	// NOTE: ext2 honors fsuid when checking for resource overrides, so
	//   you can override using fsuid too
	// Override size restrictions on IPC message queues
	//   Allow more than 64hz interrupts from the real-time clock
	// Override max number of consoles on console allocation
	// Override max number of keymaps
	CapSysResource KernelCapability = 1 << 24
	// CapSysTime :
	//   Allow manipulation of system clock
	//   Allow irix_stime on mips
	//   Allow setting the real-time clock
	CapSysTime KernelCapability = 1 << 25
	// CapSysTtyConfig :
	//   Allow configuration of tty devices
	//   Allow vhangup() of tty
	CapSysTtyConfig KernelCapability = 1 << 26
	// CapMknod :
	//   Allow the privileged aspects of mknod()
	CapMknod KernelCapability = 1 << 27
	// CapLease :
	//   Allow taking of leases on files
	CapLease KernelCapability = 1 << 28
	// CapAuditWrite :
	//   Allow writing the audit log via unicast netlink socket
	CapAuditWrite KernelCapability = 1 << 29
	// CapAuditControl :
	//   Allow configuration of audit via unicast netlink socket
	CapAuditControl KernelCapability = 1 << 30
	// CapSetfcap :
	CapSetfcap KernelCapability = 1 << 31
	// CapMacOverride :
	//   Override MAC access.
	//   The base kernel enforces no MAC policy.
	//   An LSM may enforce a MAC policy, and if it does and it chooses
	//   to implement capability based overrides of that policy, this is
	//   the capability it should use to do so.
	CapMacOverride KernelCapability = 1 << 32
	// CapMacAdmin :
	//   Allow MAC configuration or state changes.
	//   The base kernel requires no MAC configuration.
	//   An LSM may enforce a MAC policy, and if it does and it chooses
	//   to implement capability based checks on modifications to that
	//   policy or the data required to maintain it, this is the
	//   capability it should use to do so.
	CapMacAdmin KernelCapability = 1 << 33
	// CapSyslog :
	//   Allow configuring the kernel's syslog (printk behaviour)
	CapSyslog KernelCapability = 1 << 34
	// CapWakeAlarm :
	//   Allow triggering something that will wake the system
	CapWakeAlarm KernelCapability = 1 << 35
	// CapBlockSuspend :
	//   Allow preventing system suspends
	CapBlockSuspend KernelCapability = 1 << 36
	// CapAuditRead :
	//   Allow reading the audit log via multicast netlink socket
	CapAuditRead KernelCapability = 1 << 37
)

// KernelCapabilityToStrings - Kernel capability flag to string list
func KernelCapabilityToStrings(cap uint64) []string {
	rep := []string{}
	kCap := KernelCapability(cap)
	if kCap&CapChown == CapChown {
		rep = append(rep, "CapChown")
	}
	if kCap&CapDacOverride == CapDacOverride {
		rep = append(rep, "CapDacOverride")
	}
	if kCap&CapDacReadSearch == CapDacReadSearch {
		rep = append(rep, "CapDacReadSearch")
	}
	if kCap&CapFowner == CapFowner {
		rep = append(rep, "CapFowner")
	}
	if kCap&CapFsetid == CapFsetid {
		rep = append(rep, "CapFsetid")
	}
	if kCap&CapKill == CapKill {
		rep = append(rep, "CapKill")
	}
	if kCap&CapSetgid == CapSetgid {
		rep = append(rep, "CapSetgid")
	}
	if kCap&CapSetuid == CapSetuid {
		rep = append(rep, "CapSetuid")
	}
	if kCap&CapSetpcap == CapSetpcap {
		rep = append(rep, "CapSetpcap")
	}
	if kCap&CapLinuxImmutable == CapLinuxImmutable {
		rep = append(rep, "CapLinuxImmutable")
	}
	if kCap&CapNetBindService == CapNetBindService {
		rep = append(rep, "CapNetBindService")
	}
	if kCap&CapNetBroadcast == CapNetBroadcast {
		rep = append(rep, "CapNetBroadcast")
	}
	if kCap&CapNetAdmin == CapNetAdmin {
		rep = append(rep, "CapNetAdmin")
	}
	if kCap&CapNetRaw == CapNetRaw {
		rep = append(rep, "CapNetRaw")
	}
	if kCap&CapIpcLock == CapIpcLock {
		rep = append(rep, "CapIpcLock")
	}
	if kCap&CapIpcOwner == CapIpcOwner {
		rep = append(rep, "CapIpcOwner")
	}
	if kCap&CapSysModule == CapSysModule {
		rep = append(rep, "CapSysModule")
	}
	if kCap&CapSysRawio == CapSysRawio {
		rep = append(rep, "CapSysRawio")
	}
	if kCap&CapSysChroot == CapSysChroot {
		rep = append(rep, "CapSysChroot")
	}
	if kCap&CapSysPtrace == CapSysPtrace {
		rep = append(rep, "CapSysPtrace")
	}
	if kCap&CapSysPacct == CapSysPacct {
		rep = append(rep, "CapSysPacct")
	}
	if kCap&CapSysAdmin == CapSysAdmin {
		rep = append(rep, "CapSysAdmin")
	}
	if kCap&CapSysBoot == CapSysBoot {
		rep = append(rep, "CapSysBoot")
	}
	if kCap&CapSysNice == CapSysNice {
		rep = append(rep, "CapSysNice")
	}
	if kCap&CapSysResource == CapSysResource {
		rep = append(rep, "CapSysResource")
	}
	if kCap&CapSysTime == CapSysTime {
		rep = append(rep, "CapSysTime")
	}
	if kCap&CapSysTtyConfig == CapSysTtyConfig {
		rep = append(rep, "CapSysTtyConfig")
	}
	if kCap&CapMknod == CapMknod {
		rep = append(rep, "CapMknod")
	}
	if kCap&CapLease == CapLease {
		rep = append(rep, "CapLease")
	}
	if kCap&CapAuditWrite == CapAuditWrite {
		rep = append(rep, "CapAuditWrite")
	}
	if kCap&CapAuditControl == CapAuditControl {
		rep = append(rep, "CapAuditControl")
	}
	if kCap&CapSetfcap == CapSetfcap {
		rep = append(rep, "CapSetfcap")
	}
	if kCap&CapMacOverride == CapMacOverride {
		rep = append(rep, "CapMacOverride")
	}
	if kCap&CapMacAdmin == CapMacAdmin {
		rep = append(rep, "CapMacAdmin")
	}
	if kCap&CapSyslog == CapSyslog {
		rep = append(rep, "CapSyslog")
	}
	if kCap&CapWakeAlarm == CapWakeAlarm {
		rep = append(rep, "CapWakeAlarm")
	}
	if kCap&CapBlockSuspend == CapBlockSuspend {
		rep = append(rep, "CapBlockSuspend")
	}
	if kCap&CapAuditRead == CapAuditRead {
		rep = append(rep, "CapAuditRead")
	}
	if len(rep) == 0 {
		rep = []string{"None"}
	}
	return rep
}

// CloneFlag - Clone Flag
type CloneFlag uint64

const (
	// Csignal -  signal mask to be sent at exit
	Csignal CloneFlag = 1 << 7
	// CloneVM -  set if VM shared between processes
	CloneVM CloneFlag = 1 << 8
	// CloneFs -  set if fs info shared between processes
	CloneFs CloneFlag = 1 << 9
	// CloneFiles -  set if open files shared between processes
	CloneFiles CloneFlag = 1 << 10
	// CloneSighand -  set if signal handlers and blocked signals shared
	CloneSighand CloneFlag = 1 << 11
	// ClonePtrace -  set if we want to let tracing continue on the child too
	ClonePtrace CloneFlag = 1 << 13
	// CloneVfork -  set if the parent wants the child to wake it up on mm_release
	CloneVfork CloneFlag = 1 << 14
	// CloneParent -  set if we want to have the same parent as the cloner
	CloneParent CloneFlag = 1 << 15
	// CloneThread -  Same thread group?
	CloneThread CloneFlag = 1 << 16
	// CloneNewns -  New mount namespace group
	CloneNewns CloneFlag = 1 << 17
	// CloneSysvsem -  share system V SEM_UNDO semantics
	CloneSysvsem CloneFlag = 1 << 18
	// CloneSettls -  create a new TLS for the child
	CloneSettls CloneFlag = 1 << 19
	// CloneParentSettid -  set the TID in the parent
	CloneParentSettid CloneFlag = 1 << 20
	// CloneChildCleartid -  clear the TID in the child
	CloneChildCleartid CloneFlag = 1 << 21
	// CloneDetached -  Unused, ignored
	CloneDetached CloneFlag = 1 << 22
	// CloneUntraced -  set if the tracing process can't force CLONE_PTRACE on this clone
	CloneUntraced CloneFlag = 1 << 23
	// CloneChildSettid -  set the TID in the child
	CloneChildSettid CloneFlag = 1 << 24
	// CloneNewcgroup -  New cgroup namespace
	CloneNewcgroup CloneFlag = 1 << 25
	// CloneNewuts -  New utsname namespace
	CloneNewuts CloneFlag = 1 << 26
	// CloneNewipc -  New ipc namespace
	CloneNewipc CloneFlag = 1 << 27
	// CloneNewuser -  New user namespace
	CloneNewuser CloneFlag = 1 << 28
	// CloneNewpid -  New pid namespace
	CloneNewpid CloneFlag = 1 << 29
	// CloneNewnet -  New network namespace
	CloneNewnet CloneFlag = 1 << 30
	// CloneIo -  Clone io context
	CloneIo CloneFlag = 1 << 31
)

// CloneFlagToStrings - Returns the string list representation of a clone flag
func CloneFlagToStrings(input uint64) []string {
	flag := CloneFlag(input)
	rep := []string{}
	if flag&Csignal == Csignal {
		rep = append(rep, "Csignal")
	}
	if flag&CloneVM == CloneVM {
		rep = append(rep, "CloneVM")
	}
	if flag&CloneFs == CloneFs {
		rep = append(rep, "CloneFs")
	}
	if flag&CloneFiles == CloneFiles {
		rep = append(rep, "CloneFiles")
	}
	if flag&CloneSighand == CloneSighand {
		rep = append(rep, "CloneSighand")
	}
	if flag&ClonePtrace == ClonePtrace {
		rep = append(rep, "ClonePtrace")
	}
	if flag&CloneVfork == CloneVfork {
		rep = append(rep, "CloneVfork")
	}
	if flag&CloneParent == CloneParent {
		rep = append(rep, "CloneParent")
	}
	if flag&CloneThread == CloneThread {
		rep = append(rep, "CloneThread")
	}
	if flag&CloneNewns == CloneNewns {
		rep = append(rep, "CloneNewns")
	}
	if flag&CloneSysvsem == CloneSysvsem {
		rep = append(rep, "CloneSysvsem")
	}
	if flag&CloneSettls == CloneSettls {
		rep = append(rep, "CloneSettls")
	}
	if flag&CloneParentSettid == CloneParentSettid {
		rep = append(rep, "CloneParentSettid")
	}
	if flag&CloneChildCleartid == CloneChildCleartid {
		rep = append(rep, "CloneChildCleartid")
	}
	if flag&CloneDetached == CloneDetached {
		rep = append(rep, "CloneDetached")
	}
	if flag&CloneUntraced == CloneUntraced {
		rep = append(rep, "CloneUntraced")
	}
	if flag&CloneChildSettid == CloneChildSettid {
		rep = append(rep, "CloneChildSettid")
	}
	if flag&CloneNewcgroup == CloneNewcgroup {
		rep = append(rep, "CloneNewcgroup")
	}
	if flag&CloneNewuts == CloneNewuts {
		rep = append(rep, "CloneNewuts")
	}
	if flag&CloneNewipc == CloneNewipc {
		rep = append(rep, "CloneNewipc")
	}
	if flag&CloneNewuser == CloneNewuser {
		rep = append(rep, "CloneNewuser")
	}
	if flag&CloneNewpid == CloneNewpid {
		rep = append(rep, "CloneNewpid")
	}
	if flag&CloneNewnet == CloneNewnet {
		rep = append(rep, "CloneNewnet")
	}
	if flag&CloneIo == CloneIo {
		rep = append(rep, "CloneIo")
	}
	if input&uint64(SIGCHLD) == uint64(SIGCHLD) {
		rep = append(rep, "SIGCHLD")
	}
	return rep
}
