// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

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

// EtherType formats an Ether Type number as a human-readable name.
// https://www.iana.org/assignments/ieee-802-numbers/ieee-802-numbers.xhtml
func EtherType(etherTypeNumber uint32) string {
	protoStr, ok := etherTypeMap[etherTypeNumber]
	if !ok {
		return ""
	}
	return protoStr
}
