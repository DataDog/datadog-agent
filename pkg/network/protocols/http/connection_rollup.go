// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package http

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// ConnectionAggregator provides functionality for rolling-up datapoints from
// different connections that refer to the same (client, server) pair
type ConnectionAggregator struct {
	data map[ipTuple]portValues
}

// NewConnectionAggregator returns a new instance of a `ConnectionAggregator`
func NewConnectionAggregator() *ConnectionAggregator {
	return &ConnectionAggregator{
		data: make(map[ipTuple]portValues, 1000),
	}
}

type ipTuple struct {
	al uint64
	ah uint64
	bl uint64
	bh uint64
}

type ephemeralPortSide uint8

const (
	ephemeralPortUnknown = iota
	ephemeralPortA
	ephemeralPortB
)

type portValues struct {
	// these two values are populated by the *first* connection
	// that has a given ipTuple
	a uint16
	b uint16

	// this value is determined by analyzing the second connection that matches
	// the associated ipTuple but has either a different port "a" or "b"
	ephemeralSide ephemeralPortSide

	// this is used to determine whether we should generate (a, b) or (b, a)
	// when calling generateKey()
	flipped bool
}

// RollupKey returns a _potentially_ modified key that is suitable for
// aggregating datapoints belonging to the same (client, server) pair. On a
// high-level, the function is supposed to return *one* key for all connections
// matching (IP-A:*,IP-B:SERVER_PORT).
//
// This means that RollupKey(c1) == RollupKey(c2) == RollupKey(c3) for the
// example below:
//
// c1: (IP-A:60001, IP-B:8080)
// c2: (IP-A:60002, IP-B:8080)
// c3: (IP-B:8080, IP-A:60003)
//
// The approach is very simplistic and only aims to address the common cases we
// see in most workloads. The function will likely generate "false negatives",
// but ideally it shouldn't generate "false positives". In other words, we may
// not always rollup two different `types.ConnectionKey` that may legitimately
// refer to the same (client, server) pair, but we shouldn't cause correctness
// issues by aggregating two datapoints that *don't* belong to to the same
// (client, server) pair.
//
// For the sake of documentation these are the edge-cases we *don't* intend to
// address at this time:
//
// * Bare process monitoring (this code shouldn't even be reached when bare
// process monitoring is enabled);
// * Gossip/p2p protocols where you see symmetrical traffic like
// (IP-A:PORT-X,IP-B:PORT-Y) and (IP-B:PORT-X,IP-A:PORT-Y);
// * Containers running multiple processes instances bound to different ports. A
// real world example we have here at Datadog are redis containers running
// multiple instances. In this case only one one of the redis instances would
// trigger a rollup.
func (c *ConnectionAggregator) RollupKey(key types.ConnectionKey) types.ConnectionKey {
	if c == nil {
		return key
	}

	// order IPs and ports such that
	// srcIP < dstIP
	//
	// why do we do this? we want to be able to index and lookup IPs in a
	// deterministic way, but our kernel code only does a best-effort in
	// ordering tuples based on the *port* number, which sometimes generates
	// tuples that look like
	//
	// A:5000 B:8080
	// B:8080 B:9000
	//
	// when host(A) or host(B) don't really "abide" to the correct port ranges.
	normalizedKey, flipped := normalizeByIP(key)

	ips, ports := split(normalizedKey)
	savedPorts, ok := c.data[ips]
	if !ok {
		// first time we see this ipTuple, so we just save the IPs and ports as
		// we can't determine which side has the ephemeral port yet. Note that this
		// key will be returned for future connections that match
		//
		// (IP-A:PORT-A,IP-B:PORT-B)
		// (IP-A:*,IP-B:PORT-B)
		// (IP-A:PORT-A,IP-B:*)
		//
		// So let's say we see:
		// c1: A:6000 B:80
		// Followed by:
		// c2: A:6001 B:80
		// And then followed by:
		// c3: B:80 A:6002
		//
		// What we'll see is the following:
		//
		// RollupKey(c1) => c1
		// RollupKey(c2) => c1
		// RollupKey(c3) => c1
		ports.flipped = flipped
		c.data[ips] = ports
		return key
	}

	if savedPorts.a == ports.a && savedPorts.b == ports.b {
		// we're seeing the same connection for a second time, so we bail out
		// earlier. the only reason why we call `generateKey` (as opposed to
		// just `return key`) is because we want to make sure that if we first
		// saw (A:X, B:Y) and then (B:Y, A:X), we'll preserve the original key
		// ordering (A:X, B:Y)
		return generateKey(ips, savedPorts)
	}

	if savedPorts.a != ports.a && savedPorts.b != ports.b {
		// this is more like an edge-case but it may happen. in this case we
		// don't attempt to rollup the key because we would be likely be
		// merging two connections that are hitting different services
		// we're talking about something like:
		//
		// c1: A:X B:Y
		// c2: A:Z B:W
		//
		// So none of the ports match and we preserve the key as it is.
		return key
	}

	if savedPorts.ephemeralSide == ephemeralPortUnknown {
		// determine which side is the ephemeral port, in case we haven't done it yet
		// this information is only used when calling `ClearEphemeralPort()`
		if savedPorts.a == ports.a {
			savedPorts.ephemeralSide = ephemeralPortB
		} else {
			savedPorts.ephemeralSide = ephemeralPortA
		}

		c.data[ips] = savedPorts
	}

	return generateKey(ips, savedPorts)
}

// ClearEphemeralPort returns a new `types.ConnectionKey` with the ephemeral
// port set to 0. This method is supposed to be called *after* `RollupKey`.
// is called on every data point that is going to be sent to the backend.
//
// Here's an example of how this is supposed to work:
// Let's say we're using `ConnectionAggregator` for the HTTP monitoring use-case
// and assume that we have processed the following HTTP requests:
//
// request1: (IP-A:6001,IP-B:80) GET /foobar
// request2: (IP-A:6002,IP-B:80) GET /foobar
//
// If connection rollups are enabled this will produce a single aggregation:
//
// aggregation: (IP-A:6001,IP-B:80) GET /foobar [request_count=2]
//
// Note that the ephemeral port in this case happens to be 60001, simply
// because that was the first port seen in all (IP-A:*,IP-B:80) requests.
//
// The purpose of this method is to help with re-indexing the data such that NPM
// can correctly find the aggregated points. So basically, we will replace
// (IP-A:6001,IP-B:80) by (IP-A:0,IP-B:80), such that, either c1
// (IP-A:6001,IP-B:80) *or* c2 (IP-A:6002,IP-B:80), can bind to it by doing a
// (IP-A:0,IP-B:80) lookup.
// Note our encoding code makes sure that only 1 connection can claim each
// aggregation)
//
// This has the side benefit of reducing the number of orphan USM aggregations,
// because as long as *one* connection matching (IP-A:*,IP-B:80) is captured by
// NPM, all data points from USM will be sent to the backend.
func (c *ConnectionAggregator) ClearEphemeralPort(key types.ConnectionKey) types.ConnectionKey {
	if c == nil {
		return key
	}

	normalizedKey, _ := normalizeByIP(key)
	ips, ports := split(normalizedKey)
	savedPorts, ok := c.data[ips]
	if !ok || savedPorts.ephemeralSide == ephemeralPortUnknown ||
		(ports.a != savedPorts.a && ports.b != savedPorts.b) {
		// We either haven't seen at this connection, or were not able to
		// determine the ephemeral port side. In this case we return the key
		// completely unmodified.
		return key
	}

	// Get the server port from our stored information
	serverPort := savedPorts.a
	if savedPorts.ephemeralSide == ephemeralPortA {
		serverPort = savedPorts.b
	}

	// Clear the ephemeral port side
	if key.DstPort == serverPort {
		key.SrcPort = 0
	} else {
		key.DstPort = 0
	}

	return key
}

// normalizeByIP such that srcIP < dstIP
func normalizeByIP(key types.ConnectionKey) (normalizedKey types.ConnectionKey, flipped bool) {
	if key.SrcIPHigh > key.DstIPHigh || (key.SrcIPHigh == key.DstIPHigh && key.SrcIPLow > key.DstIPLow) {
		return flipKey(key), true
	}

	return key, false
}

func split(key types.ConnectionKey) (ipTuple, portValues) {
	ips := ipTuple{
		al: key.SrcIPLow,
		ah: key.SrcIPHigh,
		bl: key.DstIPLow,
		bh: key.DstIPHigh,
	}

	ports := portValues{
		a: key.SrcPort,
		b: key.DstPort,
	}

	return ips, ports
}

func generateKey(ips ipTuple, ports portValues) types.ConnectionKey {
	key := types.ConnectionKey{
		SrcIPLow:  ips.al,
		SrcIPHigh: ips.ah,
		DstIPLow:  ips.bl,
		DstIPHigh: ips.bh,
		SrcPort:   ports.a,
		DstPort:   ports.b,
	}

	if ports.flipped {
		key = flipKey(key)
	}

	return key
}

func flipKey(key types.ConnectionKey) types.ConnectionKey {
	return types.ConnectionKey{
		SrcIPLow:  key.DstIPLow,
		SrcIPHigh: key.DstIPHigh,
		DstIPLow:  key.SrcIPLow,
		DstIPHigh: key.SrcIPHigh,
		SrcPort:   key.DstPort,
		DstPort:   key.SrcPort,
	}
}
