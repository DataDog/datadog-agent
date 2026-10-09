// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && test

package workload

import (
	"net/netip"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// TestDefaultCasesRun runs every case without a tracer, checking that the
// traffic itself succeeds and that both sides of each exchange are recorded
func TestDefaultCasesRun(t *testing.T) {
	rec := Run(t, Default())
	if t.Failed() {
		return
	}

	exps := rec.Expectations()
	require.NotEmpty(t, exps)
	require.Zero(t, len(exps)%2, "expectations should come in client/server pairs")

	oracle, filter := rec.Oracle(), rec.Filter()
	for _, e := range exps {
		c := &network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Type:   e.Type,
			Source: util.Address{Addr: e.Local.Addr()},
			SPort:  e.Local.Port(),
			Dest:   util.Address{Addr: e.Remote.Addr()},
			DPort:  e.Remote.Port(),
		}}
		want, ok := oracle(c)
		if assert.True(t, ok, "no oracle entry for %s -> %s", e.Local, e.Remote) {
			assert.Equal(t, e.Want, want)
		}
		assert.True(t, filter(c))
	}
}

func TestOracleMatchesTracerTuple(t *testing.T) {
	rec := NewRecorder()
	client, server := netip.MustParseAddrPort("127.0.0.1:40000"), netip.MustParseAddrPort("127.0.0.1:8080")
	rec.ExpectPair(network.TCP, client, server, tcpBytes(10, 20), tcpBytes(20, 10))

	oracle := rec.Oracle()
	conn := func(typ network.ConnectionType, src, dst netip.AddrPort) *network.ConnectionStats {
		return &network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
			Type: typ, Source: util.AddressFromString(src.Addr().String()), SPort: src.Port(),
			Dest: util.AddressFromString(dst.Addr().String()), DPort: dst.Port(),
		}}
	}

	want, ok := oracle(conn(network.TCP, client, server))
	require.True(t, ok)
	assert.Equal(t, tcpBytes(10, 20), want)

	want, ok = oracle(conn(network.TCP, server, client))
	require.True(t, ok)
	assert.Equal(t, tcpBytes(20, 10), want)

	_, ok = oracle(conn(network.UDP, client, server))
	assert.False(t, ok, "type is part of the key")

	assert.False(t, rec.Filter()(conn(network.TCP, netip.MustParseAddrPort("127.0.0.1:1"), netip.MustParseAddrPort("127.0.0.1:2"))))
}
