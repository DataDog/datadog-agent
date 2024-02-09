// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package format

import "net"

// IPAddr formats an IP address as a string.
// If the given bytes are not a valid IP address, the behavior is undefined.
func IPAddr(ip []byte) string {
	if len(ip) == 0 {
		return ""
	}
	return net.IP(ip).String()
}
