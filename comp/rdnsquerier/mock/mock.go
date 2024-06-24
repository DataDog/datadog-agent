// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package mock provides the rdnsquerier mock component
package mock

import (
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

func (q *rdnsQuerierMock) GetHostname(ipAddr []byte, updateHostname func(string)) {
	ipaddr, ok := netip.AddrFromSlice(ipAddr)
	if !ok || !ipaddr.IsPrivate() {
		return
	}

	go func() {
		updateHostname("hostname-" + ipaddr.String())
	}()
}
