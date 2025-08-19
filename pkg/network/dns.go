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
func DNSKey(c *ConnectionStats) (dns.Key, bool) {
	if c == nil || c.DPort != 53 {
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
