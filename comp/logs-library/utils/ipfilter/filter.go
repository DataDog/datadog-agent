// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ipfilter provides IP-based allow/deny filtering for network listeners.
package ipfilter

import (
	"fmt"
	"net"
	"net/netip"
)

// Filter evaluates incoming connection addresses against allow and deny lists.
//
// Evaluation order (standard firewall semantics):
//  1. If the IP matches any denied prefix, reject.
//  2. If the allow list is non-empty and the IP matches no allowed prefix, reject.
//  3. Otherwise, accept.
type Filter struct {
	allowed []netip.Prefix
	denied  []netip.Prefix
}

// New parses the allow and deny string slices into a Filter. Each entry may be
// a bare IP address (e.g. "10.0.0.1") or CIDR notation (e.g. "10.0.0.0/24").
func New(allowed, denied []string) (*Filter, error) {
	a, err := parsePrefixes(allowed)
	if err != nil {
		return nil, fmt.Errorf("allowed_ips: %w", err)
	}
	d, err := parsePrefixes(denied)
	if err != nil {
		return nil, fmt.Errorf("denied_ips: %w", err)
	}
	return &Filter{allowed: a, denied: d}, nil
}

// Allow returns true if the address should be permitted.
func (f *Filter) Allow(addr net.Addr) bool {
	ip := addrToIP(addr)
	if !ip.IsValid() {
		return false
	}
	for _, prefix := range f.denied {
		if prefix.Contains(ip) {
			return false
		}
	}
	if len(f.allowed) == 0 {
		return true
	}
	for _, prefix := range f.allowed {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

// Active returns true if at least one allow or deny rule is configured.
func (f *Filter) Active() bool {
	return len(f.allowed) > 0 || len(f.denied) > 0
}

func parsePrefixes(entries []string) ([]netip.Prefix, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	prefixes := make([]netip.Prefix, 0, len(entries))
	for _, entry := range entries {
		prefix, err := netip.ParsePrefix(entry)
		if err != nil {
			addr, addrErr := netip.ParseAddr(entry)
			if addrErr != nil {
				return nil, fmt.Errorf("invalid IP or CIDR %q: %w", entry, err)
			}
			prefix = netip.PrefixFrom(addr, addr.BitLen())
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func addrToIP(addr net.Addr) netip.Addr {
	switch a := addr.(type) {
	case *net.TCPAddr:
		ip, _ := netip.AddrFromSlice(a.IP)
		return ip.Unmap()
	case *net.UDPAddr:
		ip, _ := netip.AddrFromSlice(a.IP)
		return ip.Unmap()
	default:
		ap, err := netip.ParseAddrPort(addr.String())
		if err != nil {
			return netip.Addr{}
		}
		return ap.Addr().Unmap()
	}
}
