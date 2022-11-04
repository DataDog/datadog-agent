// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022 Datadog, Inc.

//go:build !go1.19
// +build !go1.19

package httpsec

import "inet.af/netaddr"

type netaddrIP = netaddr.IP
type netaddrIPPrefix = netaddr.IPPrefix

var (
	netaddrParseIP       = netaddr.ParseIP
	netaddrParseIPPrefix = netaddr.ParseIPPrefix
	netaddrMustParseIP   = netaddr.MustParseIP
	netaddrIPv4          = netaddr.IPv4
	netaddrIPv6Raw       = netaddr.IPv6Raw
)
