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

// GetHostnameAsync simulates resolving the hostname for the given IP address using the default private-only lookup behavior.
func (q *rdnsQuerierMock) GetHostnameAsync(ipAddr []byte, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
	return q.GetHostnameAsyncWithOptions(ipAddr, rdnsquerier.LookupOptions{}, updateHostnameSync, updateHostnameAsync)
}

// GetHostnameAsyncWithOptions simulates resolving the hostname for the given IP address.
func (q *rdnsQuerierMock) GetHostnameAsyncWithOptions(ipAddr []byte, opts rdnsquerier.LookupOptions, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok {
		return fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !allowLookup(ipaddr, opts) {
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

// GetHostname simulates resolving the hostname for the given IP address synchronously using the default private-only lookup behavior.
func (q *rdnsQuerierMock) GetHostname(ctx context.Context, ipAddr string) (string, error) {
	return q.GetHostnameWithOptions(ctx, ipAddr, rdnsquerier.LookupOptions{})
}

// GetHostnameWithOptions simulates resolving the hostname for the given IP address synchronously.
func (q *rdnsQuerierMock) GetHostnameWithOptions(_ context.Context, ipAddr string, opts rdnsquerier.LookupOptions) (string, error) {
	netipAddr, err := netip.ParseAddr(ipAddr)
	if err != nil {
		return "", fmt.Errorf("invalid IP address %v", ipAddr)
	}

	if !allowLookup(netipAddr, opts) {
		return "", nil
	}

	return "hostname-" + netipAddr.String(), nil
}

// GetHostnames simulates resolving the hostnames for the given IP addresses synchronously using the default private-only lookup behavior.
func (q *rdnsQuerierMock) GetHostnames(ctx context.Context, ipAddrs []string) map[string]rdnsquerier.ReverseDNSResult {
	return q.GetHostnamesWithOptions(ctx, ipAddrs, rdnsquerier.LookupOptions{})
}

// GetHostnamesWithOptions simulates resolving the hostnames for the given IP addresses synchronously.
func (q *rdnsQuerierMock) GetHostnamesWithOptions(_ context.Context, ipAddrs []string, opts rdnsquerier.LookupOptions) map[string]rdnsquerier.ReverseDNSResult {
	results := make(map[string]rdnsquerier.ReverseDNSResult, len(ipAddrs))
	for _, ipAddr := range ipAddrs {
		netipAddr, err := netip.ParseAddr(ipAddr)
		if err != nil {
			results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr, Err: fmt.Errorf("invalid IP address %v", ipAddr)}
			continue
		}

		if !allowLookup(netipAddr, opts) {
			results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr}
			continue
		}

		results[ipAddr] = rdnsquerier.ReverseDNSResult{IP: ipAddr, Hostname: "hostname-" + netipAddr.String()}
	}

	return results
}

func allowLookup(addr netip.Addr, opts rdnsquerier.LookupOptions) bool {
	return addr.IsPrivate() || opts.AllowPublic
}
