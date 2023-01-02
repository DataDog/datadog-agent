// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package httpsec

import "net/netip"

func netaddrIPv4(a, b, c, d byte) netip.Addr {
	e := [4]byte{a, b, c, d}
	return netip.AddrFrom4(e)
}
