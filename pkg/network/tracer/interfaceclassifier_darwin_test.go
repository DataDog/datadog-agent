// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInterfaceTypeFor(t *testing.T) {
	cases := []struct {
		name  string
		iface string
		flags net.Flags
		want  string
	}{
		{name: "loopback flag", iface: "lo0", flags: net.FlagLoopback, want: "software_loopback"},
		{name: "lo prefix", iface: "lo0", want: "software_loopback"},
		{name: "loopback flag wins over en", iface: "en0", flags: net.FlagLoopback, want: "software_loopback"},
		{name: "utun", iface: "utun3", want: "tunnel"},
		{name: "gif", iface: "gif0", want: "tunnel"},
		{name: "stf", iface: "stf0", want: "tunnel"},
		{name: "ipsec", iface: "ipsec0", want: "tunnel"},
		{name: "ppp", iface: "ppp0", want: "tunnel"},
		{name: "bridge", iface: "bridge0", want: "bridge"},
		{name: "vlan", iface: "vlan1", want: "vlan"},
		{name: "en", iface: "en0", want: "ethernet_csmacd"},
		{name: "other", iface: "awdl0", want: "other"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, interfaceTypeFor(tc.iface, tc.flags))
		})
	}
}

func TestClassify_UsesCachedName(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			14: {name: "utun3", ifaceType: "tunnel"},
		},
		done: make(chan struct{}),
	}

	result := c.Classify(14)
	assert.Equal(t, "utun3", result.InterfaceName)
	assert.Equal(t, "tunnel", result.InterfaceType)
}

func TestClassify_ZeroAndUnknownIndex(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{
			1: {name: "lo0", ifaceType: "software_loopback"},
		},
		done: make(chan struct{}),
	}

	assert.Equal(t, InterfaceClassification{}, c.Classify(0))
	assert.Equal(t, InterfaceClassification{}, c.Classify(99))
	assert.Equal(t, InterfaceClassification{}, (*InterfaceClassifier)(nil).Classify(1))
}

func TestRefreshCacheMatchesNetInterfaces(t *testing.T) {
	c := &InterfaceClassifier{
		ifCache: map[uint32]cachedInterface{},
		done:    make(chan struct{}),
	}
	c.refreshCache()

	ifaces, err := net.Interfaces()
	require.NoError(t, err)
	seen := 0
	for _, iface := range ifaces {
		if iface.Index <= 0 || iface.Name == "" {
			continue
		}
		seen++
		got := c.Classify(uint32(iface.Index))
		assert.Equal(t, iface.Name, got.InterfaceName)
		assert.Equal(t, interfaceTypeFor(iface.Name, iface.Flags), got.InterfaceType)
	}
	assert.NotZero(t, seen)
}

func TestNewInterfaceClassifierClose(t *testing.T) {
	c := NewInterfaceClassifier()
	require.NotNil(t, c)
	c.Close()
	c.Close()
}
