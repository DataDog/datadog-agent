// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the rdnsquerier mock component
package mock

import (
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

// GetHostname simulates resolving the hostname for the given IP address.  If the IP address is in the private address
// space then, depending on the IP address, either the updateHostnameSync callback will be invoked synchronously as if
// there was a cache hit, or the updateHostnameAsync callback will be invoked asynchronously with the simulated resolved hostname.
func (q *rdnsQuerierMock) GetHostname(ipAddr []byte, updateHostnameSync func(string), updateHostnameAsync func(string, error)) error {
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
