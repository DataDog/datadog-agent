// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && linux_bpf) || (windows && npm)

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestHTTPStats(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, []byte("/testpath"), true, http.MethodGet)

	httpStats := make(map[http.Key]*http.RequestStats)
	httpStats[key] = http.NewRequestStats()

	usmStats := make(map[protocols.ProtocolType]interface{})
	usmStats[protocols.HTTP] = httpStats

	// Register client & pass in HTTP stats
	state := newDefaultState()
	delta := state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, usmStats)

	// Verify connection has HTTP data embedded in it
	assert.Len(t, delta.USMData.HTTP, 1)

	// Verify HTTP data has been flushed
	delta = state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.HTTP, 0)
}

func TestHTTPStatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	getStats := func(path string) map[protocols.ProtocolType]interface{} {
		httpStats := make(map[http.Key]*http.RequestStats)
		key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, []byte(path), true, http.MethodGet)
		httpStats[key] = http.NewRequestStats()

		usmStats := make(map[protocols.ProtocolType]interface{})
		usmStats[protocols.HTTP] = httpStats

		return usmStats
	}

	client1 := "client1"
	client2 := "client2"
	client3 := "client3"
	state := newDefaultState()

	// Register the first two clients
	state.RegisterClient(client1)
	state.RegisterClient(client2)

	// We should have nothing on first call
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).USMData.HTTP, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).USMData.HTTP, 0)

	// Store the connection to both clients & pass HTTP stats to the first client
	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	delta := state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath"))
	assert.Len(t, delta.USMData.HTTP, 1)

	// Verify that the HTTP stats were also stored in the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.HTTP, 1)

	// Register a third client & verify that it does not have the HTTP stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.HTTP, 0)

	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	// Pass in new HTTP stats to the first client
	delta = state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath2"))
	assert.Len(t, delta.USMData.HTTP, 1)

	// And the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, getStats("/testpath3"))
	assert.Len(t, delta.USMData.HTTP, 2)

	// Verify that the third client also accumulated both new HTTP stats
	delta = state.GetDelta(client3, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.HTTP, 2)
}
