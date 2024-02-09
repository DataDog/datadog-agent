/* SPDX-License-Identifier: BSD-2-Clause */

package net

// IPProto is the IP protocol type
type IPProto int

// a few common IANA protocol numbers
var (
	ProtoICMP   IPProto = 1
	ProtoTCP    IPProto = 6
	ProtoUDP    IPProto = 17
	ProtoICMPv6 IPProto = 58
)
