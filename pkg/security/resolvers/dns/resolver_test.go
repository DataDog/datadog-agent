// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package dns resolves ip addresses to hostnames
package dns

import (
	"net/netip"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/probe/config"
)

func BenchmarkDNSResolverQuery(b *testing.B) {
	cfg := &config.Config{
		DNSResolverCacheSize: 1000,
	}
	resolver, err := NewDNSResolver(cfg, nil)
	if err != nil {
		b.Fatalf("failed to create DNS resolver: %v", err)
	}

	ip := netip.MustParseAddr("151.101.64.81")
	resolver.AddNewCname("www.bbc.co.uk.pri.bbc.co.uk", "www.bbc.co.uk")
	resolver.AddNewCname("bbc.map.fastly.net", "www.bbc.co.uk.pri.bbc.co.uk")
	resolver.AddNew("bbc.map.fastly.net", ip)
	resolver.CommitInFlights()

	for b.Loop() {
		_ = resolver.HostListFromIP(ip)
	}
}

func BenchmarkDNSResolverInsertion(b *testing.B) {
	cfg := &config.Config{
		DNSResolverCacheSize: 1000,
	}
	resolver, err := NewDNSResolver(cfg, nil)
	if err != nil {
		b.Fatalf("failed to create DNS resolver: %v", err)
	}

	ip1 := netip.MustParseAddr("151.101.64.81")
	ip2 := netip.MustParseAddr("151.101.0.81")
	ip3 := netip.MustParseAddr("151.101.192.81")
	ip4 := netip.MustParseAddr("151.101.128.81")

	for b.Loop() {
		resolver.AddNewCname("www.bbc.co.uk.pri.bbc.co.uk", "www.bbc.co.uk")
		resolver.AddNewCname("bbc.map.fastly.net", "www.bbc.co.uk.pri.bbc.co.uk")
		resolver.AddNew("bbc.map.fastly.net", ip1)
		resolver.AddNew("bbc.map.fastly.net", ip2)
		resolver.AddNew("bbc.map.fastly.net", ip3)
		resolver.AddNew("bbc.map.fastly.net", ip4)
		resolver.CommitInFlights()

		b.StopTimer()
		resolver.clear()
		b.StartTimer()
	}
}
