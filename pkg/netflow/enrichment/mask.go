// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package enrichment

import (
	"net"
	"strconv"
)

// FormatMask formats mask raw value (uint32) into CIDR format (e.g. `192.1.128.64/26`)
func FormatMask(ipAddr []byte, maskRawValue uint32) string {
	maskSuffix := "/" + strconv.Itoa(int(maskRawValue))

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

	mask := net.CIDRMask(int(maskRawValue), maskBitsLen)
	if mask == nil {
		return maskSuffix
	}
	maskedIP := ip.Mask(mask)
	if maskedIP == nil {
		return maskSuffix
	}
	return maskedIP.String() + maskSuffix
}
