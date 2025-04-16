// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package common

import (
	"net"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestUnmappedAddrFromSliceZero(t *testing.T) {
	// zero value
	addr, ok := UnmappedAddrFromSlice(nil)
	require.Equal(t, netip.Addr{}, addr)
	require.False(t, ok)
}

func TestUnmappedAddrFromSliceIPv4(t *testing.T) {
	addr, ok := UnmappedAddrFromSlice(net.ParseIP("192.168.1.1"))
	require.Equal(t, netip.MustParseAddr("192.168.1.1"), addr)
	require.True(t, ok)
}

func TestUnmappedAddrFromSliceIPv6(t *testing.T) {
	addr, ok := UnmappedAddrFromSlice(net.ParseIP("::1"))
	require.Equal(t, netip.MustParseAddr("::1"), addr)
	require.True(t, ok)
}

func TestUnmappedAddrFromSliceMappedIPv4(t *testing.T) {
	addr, ok := UnmappedAddrFromSlice(net.ParseIP("::ffff:54.146.50.212"))
	require.Equal(t, netip.MustParseAddr("54.146.50.212"), addr)
	require.True(t, ok)
}
