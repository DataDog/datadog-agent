// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the rdnsquerier mock component
package mock

import (
	"context"
	"fmt"
	"net/netip"

	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
)

// Mock implements mock-specific methods.
type Mock interface {
	rdnsquerier.Component
}

type rdnsQuerierMock struct{}

// NewMock returns a mock for the rdnsquerier component.
func NewMock() rdnsquerier.Component {
	return &rdnsQuerierMock{}
}

// GetHostnameAsync simulates resolving the hostname for the given IP address.  If the IP address is in the private address
// space then, depending on the IP address, either the updateHostnameSync callback will be invoked synchronously as if
// there was a cache hit, or the updateHostnameAsync callback will be invoked asynchronously with the simulated resolved hostname.
func (q *rdnsQuerierMock) GetHostnameAsync(ipAddr []byte, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		return fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !ipaddr.IsPrivate() {
		return nil
	}

	if (ipAddr[3] / 10 % 2) == 0 {
		updateHostnameSync("hostname-" + ipaddr.String())
		return nil
	}

	go func() {
		updateHostnameAsync("hostname-"+ipaddr.String(), nil)
	}()

	return nil
}

// GetHostname simulates resolving the hostname for the given IP address synchronously.  If the IP address is in the private address
// space then the resolved hostname is returned.
func (q *rdnsQuerierMock) GetHostname(_ context.Context, ipAddr string) (string, error) {
	netipAddr, err := netip.ParseAddr(ipAddr)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !netipAddr.IsPrivate() {
		return "", nil
	}

	return "hostname-" + netipAddr.String(), nil
}

// GetHostnames simulates resolving the hostnames for the given IP addresses synchronously.  If the IP address is in the private address
// space then the resolved hostname is returned.
func (q *rdnsQuerierMock) GetHostnames(_ context.Context, ipAddrs []string) map[string]rdnsquerier.ReverseDNSResult {
	results := make(map[string]rdnsquerier.ReverseDNSResult, len(ipAddrs))
	for _, ipAddr := range ipAddrs {
		netipAddr, err := netip.ParseAddr(ipAddr)
		if err != nil {
			results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr, Err: fmt.Errorf("invalid IP address %v", ipAddr)}
			continue
		}

		if !netipAddr.IsPrivate() {
			results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr}
			continue
		}

		results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr, Hostname: "hostname-" + netipAddr.String()}
	}

	return results
}
