package common

import (
	"encoding/binary"
	"fmt"
	"github.com/golang/protobuf/proto"
	"net"
	"reflect"
	"strings"
)

const (
	// FormatTypeUnknown desc
	FormatTypeUnknown = iota
	// FormatTypeStringFunc desc
	FormatTypeStringFunc
	// FormatTypeString desc
	FormatTypeString
	// FormatTypeInteger desc
	FormatTypeInteger
	// FormatTypeIP desc
	FormatTypeIP
	// FormatTypeMac desc
	FormatTypeMac
	// FormatTypeBytes desc
	FormatTypeBytes
)

var (
	// EtypeName desc
	EtypeName = map[uint32]string{
		0x806:  "ARP",
		0x800:  "IPv4",
		0x86dd: "IPv6",
	}
	// ProtoName desc
	ProtoName = map[uint32]string{
		1:   "ICMP",
		6:   "TCP",
		17:  "UDP",
		58:  "ICMPv6",
		132: "SCTP",
	}
	// L7ProtoName desc
	L7ProtoName = map[uint32]string{
		21:  "ftp",
		22:  "ssh",
		53:  "dns",
		67:  "dhcp_server",
		68:  "dhcp_client",
		80:  "http",
		123: "ntp",
		137: "netbios_name",
		138: "netbios_datagram",
		443: "https",
		587: "smtp",
		771: "rtip",
	}
	// IcmpTypeName desc
	IcmpTypeName = map[uint32]string{
		0:  "EchoReply",
		3:  "DestinationUnreachable",
		8:  "Echo",
		9:  "RouterAdvertisement",
		10: "RouterSolicitation",
		11: "TimeExceeded",
	}
	// Icmp6TypeName desc
	Icmp6TypeName = map[uint32]string{
		1:   "DestinationUnreachable",
		2:   "PacketTooBig",
		3:   "TimeExceeded",
		128: "EchoRequest",
		129: "EchoReply",
		133: "RouterSolicitation",
		134: "RouterAdvertisement",
	}

	// TextFields desc
	TextFields = []string{
		"Type",
		"TimeReceived",
		"SequenceNum",
		"SamplingRate",
		"SamplerAddress",
		"TimeFlowStart",
		"TimeFlowEnd",
		"Bytes",
		"Packets",
		"SrcAddr",
		"DstAddr",
		"Etype",
		"Proto",
		"SrcPort",
		"DstPort",
		"InIf",
		"OutIf",
		"SrcMac",
		"DstMac",
		"SrcVlan",
		"DstVlan",
		"VlanId",
		"IngressVrfID",
		"EgressVrfID",
		"IPTos",
		"ForwardingStatus",
		"IPTTL",
		"TCPFlags",
		"IcmpType",
		"IcmpCode",
		"IPv6FlowLabel",
		"FragmentId",
		"FragmentOffset",
		"BiFlowDirection",
		"SrcAS",
		"DstAS",
		"NextHop",
		"NextHopAS",
		"SrcNet",
		"DstNet",
	}
	// TextFieldsTypes desc
	TextFieldsTypes = []int{
		FormatTypeStringFunc,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeIP,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeIP,
		FormatTypeIP,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeMac,
		FormatTypeMac,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeIP,
		FormatTypeInteger,
		FormatTypeInteger,
		FormatTypeInteger,
	}
	// RenderExtras desc
	RenderExtras = []string{
		"EtypeName",
		"ProtoName",
		"IcmpName",
	}
	// RenderExtraCall desc
	RenderExtraCall = []RenderExtraFunction{
		RenderExtraFunctionEtypeName,
		RenderExtraFunctionProtoName,
		RenderExtraFunctionIcmpName,
	}
)

// AddTextField desc
// AddTextField desc
func AddTextField(name string, jtype int) {
	TextFields = append(TextFields, name)
	TextFieldsTypes = append(TextFieldsTypes, jtype)
}

// RenderExtraFunction desc
type RenderExtraFunction func(proto.Message) string

// RenderExtraFetchNumbers desc
func RenderExtraFetchNumbers(msg proto.Message, fields []string) []uint64 {
	vfm := reflect.ValueOf(msg)
	vfm = reflect.Indirect(vfm)

	values := make([]uint64, len(fields))
	for i, kf := range fields {
		fieldValue := vfm.FieldByName(kf)
		if fieldValue.IsValid() {
			values[i] = fieldValue.Uint()
		}
	}

	return values
}

// RenderExtraFunctionEtypeName desc
func RenderExtraFunctionEtypeName(msg proto.Message) string {
	num := RenderExtraFetchNumbers(msg, []string{"Etype"})
	return EtypeName[uint32(num[0])]
}

// RenderExtraFunctionProtoName desc
func RenderExtraFunctionProtoName(msg proto.Message) string {
	num := RenderExtraFetchNumbers(msg, []string{"Proto"})
	return ProtoName[uint32(num[0])]
}

// RenderExtraFunctionIcmpName desc
func RenderExtraFunctionIcmpName(msg proto.Message) string {
	num := RenderExtraFetchNumbers(msg, []string{"Proto", "IcmpCode", "IcmpType"})
	return IcmpCodeType(uint32(num[0]), uint32(num[1]), uint32(num[2]))
}

// IcmpCodeType desc
func IcmpCodeType(proto, icmpCode, icmpType uint32) string {
	if proto == 1 {
		return IcmpTypeName[icmpType]
	} else if proto == 58 {
		return Icmp6TypeName[icmpType]
	}
	return ""
}

// RenderIP desc
func RenderIP(addr []byte) string {
	if addr == nil || (len(addr) != 4 && len(addr) != 16) {
		return ""
	}

	return net.IP(addr).String()
}

// FormatMessageReflectText desc
func FormatMessageReflectText(msg proto.Message, ext string) string {
	return FormatMessageReflectCustom(msg, ext, "", " ", "=", false)
}

// FormatMessageReflectJSON desc
func FormatMessageReflectJSON(msg proto.Message, ext string) string {
	return fmt.Sprintf("{%s}", FormatMessageReflectCustom(msg, ext, "\"", ",", ":", true))
}

// FormatMessageReflectCustom desc
func FormatMessageReflectCustom(msg proto.Message, ext, quotes, sep, sign string, null bool) string {
	fstr := make([]string, len(TextFields)+len(RenderExtras))

	vfm := reflect.ValueOf(msg)
	vfm = reflect.Indirect(vfm)

	var i int
	for j, kf := range TextFields {
		fieldValue := vfm.FieldByName(kf)
		if fieldValue.IsValid() {

			switch TextFieldsTypes[j] {
			case FormatTypeStringFunc:
				strMethod := fieldValue.MethodByName("String").Call([]reflect.Value{})
				fstr[i] = fmt.Sprintf("%s%s%s%s%q", quotes, kf, quotes, sign, strMethod[0].String())
			case FormatTypeString:
				fstr[i] = fmt.Sprintf("%s%s%s%s%q", quotes, kf, quotes, sign, fieldValue.String())
			case FormatTypeInteger:
				fstr[i] = fmt.Sprintf("%s%s%s%s%d", quotes, kf, quotes, sign, fieldValue.Uint())
			case FormatTypeIP:
				ip := fieldValue.Bytes()
				fstr[i] = fmt.Sprintf("%s%s%s%s%q", quotes, kf, quotes, sign, RenderIP(ip))
			case FormatTypeMac:
				mac := make([]byte, 8)
				binary.BigEndian.PutUint64(mac, fieldValue.Uint())
				fstr[i] = fmt.Sprintf("%s%s%s%s%q", quotes, kf, quotes, sign, net.HardwareAddr(mac[2:]).String())
			case FormatTypeBytes:
				fstr[i] = fmt.Sprintf("%s%s%s%s%.2x", quotes, kf, quotes, sign, fieldValue.Bytes())
			default:
				if null {
					fstr[i] = fmt.Sprintf("%s%s%s%snull", quotes, kf, quotes, sign)
				}
			}

		} else {
			if null {
				fstr[i] = fmt.Sprintf("%s%s%s%snull", quotes, kf, quotes, sign)
			}
		}
		if len(selectorMap) == 0 || selectorMap[kf] {
			i++
		}

	}

	for j, e := range RenderExtras {
		fstr[i] = fmt.Sprintf("%s%s%s%s%q", quotes, e, quotes, sign, RenderExtraCall[j](msg))
		if len(selectorMap) == 0 || selectorMap[e] {
			i++
		}
	}

	if len(selectorMap) > 0 {
		fstr = fstr[0:i]
	}

	return strings.Join(fstr, sep)
}
