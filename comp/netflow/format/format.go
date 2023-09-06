// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

// Package format provides methods for converting various netflow-related
// structures and values into strings.
package format

import (
	"encoding/binary"
	"net"
	"strconv"
)

// Direction remaps direction from 0 or 1 to respectively ingress or egress
func Direction(direction uint32) string {
	if direction == 1 {
		return "egress"
	}
	return "ingress"
}

// Currently mapping only IPv4 and IPv6 since those are the main case defined in goflow2
//   - For NetFlow5/9/IPFIX, ether type can take other values if dataLinkFrameSection is defined.
//     https://github.com/netsampler/goflow2/blob/614539b9543548179fd3f168e7273c5269ec09b4/producer/producer_nf.go#L390-L391
//   - For SFlow, ether type can take other values if the protocol is Ethernet
//     https://github.com/netsampler/goflow2/blob/615b9f697cc23c22a2181adfde1005c471f62347/producer/producer_sf.go#L227-L230
//
// Support for other kind of ether type can be implemented later.
var etherTypeMap = map[uint32]string{
	0x0800: "IPv4",
	0x86DD: "IPv6",
}

// EtherType formats an EtherType number as a human-readable string
// https://www.iana.org/assignments/ieee-802-numbers/ieee-802-numbers.xhtml
func EtherType(etherTypeNumber uint32) string {
	protoStr, ok := etherTypeMap[etherTypeNumber]
	if !ok {
		return ""
	}
	return protoStr
}

// MacAddress formats a mac address as "xx:xx:xx:xx:xx:xx"
func MacAddress(fieldValue uint64) string {
	mac := make([]byte, 8)
	binary.BigEndian.PutUint64(mac, fieldValue)
	return net.HardwareAddr(mac[2:]).String()
}

// CIDR formats an IP and number of bits in CIDR format (e.g. `192.1.128.64/26`).
// ones should be the number of ones in the bitmask, e.g. 26 in the example above.
func CIDR(ipAddr []byte, ones uint32) string {
	maskSuffix := "/" + strconv.Itoa(int(ones))

	ip := net.IP(ipAddr)
	if ip == nil {
		return maskSuffix
	}

	var maskBitsLen int
	// Using ip.To4() to test for ipv4
	// More info: https://stackoverflow.com/questions/40189084/what-is-ipv6-for-localhost-and-0-0-0-0
	if ip.To4() != nil {
		maskBitsLen = 32
	} else {
		maskBitsLen = 128
	}

	mask := net.CIDRMask(int(ones), maskBitsLen)
	if mask == nil {
		return maskSuffix
	}
	maskedIP := ip.Mask(mask)
	if maskedIP == nil {
		return maskSuffix
	}
	return maskedIP.String() + maskSuffix
}

var tcpFlagsMapping = map[uint32]string{
	1:  "FIN",
	2:  "SYN",
	4:  "RST",
	8:  "PSH",
	16: "ACK",
	32: "URG",
}

// TCPFlags formats a TCP bitmask as a set of strings.
func TCPFlags(flags uint32) []string {
	var strFlags []string
	flag := uint32(1)
	for {
		if flag > 32 {
			break
		}
		strFlag, ok := tcpFlagsMapping[flag]
		if !ok {
			continue
		}
		if flag&flags != 0 {
			strFlags = append(strFlags, strFlag)
		}
		flag <<= 1
	}
	return strFlags
}

// IPProtocol maps an IP protocol number to a standard name.
// https://www.iana.org/assignments/protocol-numbers/protocol-numbers.xhtml
func IPProtocol(protocolNumber uint32) string {
	return protocolMap[protocolNumber]
}

// IPAddr formats an IP address as a string.
// If the given bytes are not a valid IP address, the behavior is undefined.
func IPAddr(ip []byte) string {
	if len(ip) == 0 {
		return ""
	}
	return net.IP(ip).String()
}

// Port formats a port number. It's the same as strconv.Itoa, except that port
// -1 is mapped to the special string '*'.
func Port(port int32) string {
	if port >= 0 {
		return strconv.Itoa(int(port))
	}
	if port == -1 {
		return "*"
	}
	// this should never happen since port is either zero/positive or -1 (ephemeral port), no other value is currently supported
	return "invalid"
}
