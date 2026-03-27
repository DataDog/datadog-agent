// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package utils holds utils related files
package utils

import "net"

// GetIPStringFromIPNet returns the string representation of the IP from an IPNet,
// returning an empty string when the IP is nil.
func GetIPStringFromIPNet(ipNet net.IPNet) string {
	if len(ipNet.IP) == 0 {
		return ""
	}
	return ipNet.IP.String()
}
