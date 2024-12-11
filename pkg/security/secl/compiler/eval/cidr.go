// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"errors"
	"net"
	"strings"
)

var (
	// IPV4Mask32 ipv4 ip address
	IPV4Mask32 = net.CIDRMask(32, 8*net.IPv4len)
	// IPV6Mask128 ipv6 ip address
	IPV6Mask128 = net.CIDRMask(128, 8*net.IPv6len)
)

// IPNetFromIP returns a IPNET version of the IP
func IPNetFromIP(ip net.IP) *net.IPNet {
	var mask = IPV4Mask32
	if len(ip) == net.IPv6len {
		mask = IPV6Mask128
	}

	return &net.IPNet{
		IP:   ip,
		Mask: mask,
	}
}

// CIDRValues describes a set of CIDRs
type CIDRValues struct {
	ipnets []*net.IPNet

	// caches
	fieldValues []FieldValue

	exists map[string]bool
}

// AppendCIDR append a CIDR notation
func (c *CIDRValues) AppendCIDR(cidr string) error {
	if c.exists[cidr] {
		return nil
	}

	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	c.ipnets = append(c.ipnets, ipnet)
	c.fieldValues = append(c.fieldValues, FieldValue{Type: IPNetValueType, Value: *ipnet})

	if c.exists == nil {
		c.exists = make(map[string]bool)
	}
	c.exists[cidr] = true

	return nil
}

func isZeros(p net.IP) bool {
	for i := 0; i < len(p); i++ {
		if p[i] != 0 {
			return false
		}
	}
	return true
}

// ParseCIDR converts an IP/CIDR notation to an IPNet object
func ParseCIDR(ip string) (*net.IPNet, error) {
	var ipnet *net.IPNet
	if !strings.Contains(ip, "/") {
		if ipnet = IPNetFromIP(net.ParseIP(ip)); isZeros(ipnet.IP) {
			return nil, errors.New("unknown IP address format")
		}
	} else {
		var err error
		if _, ipnet, err = net.ParseCIDR(ip); err != nil {
			return nil, err
		}
	}

	return ipnet, nil
}

// AppendIP append ip notation
func (c *CIDRValues) AppendIP(ip string) error {
	if c.exists[ip] {
		return nil
	}

	ipnet, err := ParseCIDR(ip)
	if err != nil {
		return err
	}

	c.ipnets = append(c.ipnets, ipnet)
	c.fieldValues = append(c.fieldValues, FieldValue{Type: IPNetValueType, Value: *ipnet})

	if c.exists == nil {
		c.exists = make(map[string]bool)
	}
	c.exists[ip] = true

	return nil
}

// Contains returns whether the values match the provided IPNet
func (c *CIDRValues) Contains(ipnet *net.IPNet) bool {
	for _, n := range c.ipnets {
		if IPNetsMatch(n, ipnet) {
			return true
		}
	}

	return false
}

// Match returns whether the values matches the provided IPNets
func (c *CIDRValues) Match(ipnets []net.IPNet) bool {
	for _, n := range c.ipnets {
		for _, ipnet := range ipnets {
			if IPNetsMatch(n, &ipnet) {
				return true
			}
		}
	}

	return false
}

// MatchAll returns whether the values matches all the provided IPNets
func (c *CIDRValues) MatchAll(ipnets []net.IPNet) bool {
	for _, n := range c.ipnets {
		for _, ipnet := range ipnets {
			if !IPNetsMatch(n, &ipnet) {
				return false
			}
		}
	}

	return true
}

// IPNetsMatch returns whether the IPNets match
func IPNetsMatch(i1, i2 *net.IPNet) bool {
	return i1.Contains(i2.IP) || i2.Contains(i1.IP)
}
