// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

//go:build go1.19
// +build go1.19

package httpsec

import "net/netip"

type netaddrIP = netip.Addr
type netaddrIPPrefix = netip.Prefix

var (
	netaddrParseIP       = netip.ParseAddr
	netaddrParseIPPrefix = netip.ParsePrefix
	netaddrMustParseIP   = netip.MustParseAddr
	netaddrIPv6Raw       = netip.AddrFrom16
)

func netaddrIPv4(a, b, c, d byte) netaddrIP {
	e := [4]byte{a, b, c, d}
	return netip.AddrFrom4(e)
}
