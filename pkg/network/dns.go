// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package network

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/network/dns"
)

// DNSKey generates a key suitable for looking up DNS stats based on a ConnectionStats object
func DNSKey(c *ConnectionStats, allowedPorts []int) (dns.Key, bool) {
	if c == nil {
		return dns.Key{}, false
	}

	isDNS := false
	for _, p := range allowedPorts {
		if c.DPort == uint16(p) {
			isDNS = true
			break
		}
	}
	if !isDNS {
		return dns.Key{}, false
	}

	serverIP, _ := GetNATRemoteAddress(*c)
	clientIP, clientPort := GetNATLocalAddress(*c)
	key := dns.Key{
		ServerIP:   serverIP,
		ClientIP:   clientIP,
		ClientPort: clientPort,
	}
	switch c.Type {
	case TCP:
		key.Protocol = syscall.IPPROTO_TCP
	case UDP:
		key.Protocol = syscall.IPPROTO_UDP
	}

	return key, true
}
