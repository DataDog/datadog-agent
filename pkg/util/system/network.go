// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package system

import (
	"fmt"
	"net"
)

// NetworkRoute holds one network destination subnet and it's linked interface name
type NetworkRoute struct {
	Interface string
	Subnet    uint64
	Gateway   uint64
	Mask      uint64
}

// IsLocalAddress returns the given address if it is local or an error if it is not
func IsLocalAddress(address string) (string, error) {
	if address == "localhost" {
		return address, nil
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "", fmt.Errorf("address was set to an invalid IP address: %s", address)
	}
	for _, cidr := range []string{
		"127.0.0.0/8", // IPv4 loopback
		"::1/128",     // IPv6 loopback
	} {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			return "", err
		}
		if block.Contains(ip) {
			return address, nil
		}
	}
	return "", fmt.Errorf("address was set to a non-loopback IP address: %s", address)
}
