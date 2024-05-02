// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package utils

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

const (
	// defaultIPTupleSize represents the default capacity of the map storing
	// (IP-A:IP-B) tuples. The number here should be more or less the number
	// of local IPs a host has times the number of remote IPs it
	// talks to.
	defaultIPTupleSize = 1000

	// defaultPortSampleSize represents the default capacity of the slice
	// storing `portTuple` objects for a given (IP-A:IP-B) tuple. This slice
	// should store one `portTuple` object per unique server port, so its
	// cardiality should be relatively low.
	// For example, a HTTP client container with (IP-A) hitting a HTTP server
	// with (IP-B), will probably have one or two `portTuple` objects (:80),
	// (:443).
	defaultPortSampleSize = 10
)

// ConnectionAggregator provides functionality for rolling-up datapoints from
// different connections that refer to the same (client, server) pair
type ConnectionAggregator struct {
	data map[ipTuple][]portTuple
}

// NewConnectionAggregator returns a new instance of a `ConnectionAggregator`
func NewConnectionAggregator() *ConnectionAggregator {
	return &ConnectionAggregator{
		data: make(map[ipTuple][]portTuple, defaultIPTupleSize),
	}
}

// ipTuple represents a pair of 64-bit IP addresses.
//
// note: we chose generic names ("a" and "b") on purpose, since the order of IPs
// here doesn't align with the notion of "source/destination", "local/remote" or
// "client/server".
type ipTuple struct {
	aLow  uint64
	aHigh uint64
	bLow  uint64
	bHigh uint64
}

// portTuple represents a "sample" of a pair of port numbers associated to a
// given ipTuple.
type portTuple struct {
	a uint16
	b uint16

	// this value is determined by analyzing *similar* portTuple values. please
	// refer to the comments further down for an explanation of what this means.
	serverSide serverPortSide
}

// serverPortSide designates the side of the tuple ("a" or "b") that contains
// the server port
type serverPortSide uint8

const (
	serverPortUnknown = iota
	serverPortA
	serverPortB
)

// RollupKey returns a _potentially_ modified key that is suitable for
// aggregating datapoints belonging to the same (client, server) pair. On a
// high-level, the function is supposed to return *one* key for all connections
// matching (IP-A:*,IP-B:SERVER_PORT).
//
// The approach here was designed such that it can be used in the context of a
// *stream* of events, so we don't need to have the full data set in order to
// aggregate different connections.
//
// Here's an input/output example:
//
// |-------------------------------*-------------------------------|
// | Input                         | Output                        |
// |-------------------------------+-------------------------------|
// | (1.1.1.1:60001 - 2.2.2.2:80)  | (1.1.1.1:60001 - 2.2.2.2:80)  |
// | (1.1.1.1:60002 - 2.2.2.2:80)  | (1.1.1.1:60001 - 2.2.2.2:80)  |
// | (1.1.1.1:60003 - 3.3.3.3:80)  | (1.1.1.1:60003 - 3.3.3.3:80)  |
// | (1.1.1.1:60004 - 2.2.2.2:443) | (1.1.1.1:60004 - 2.2.2.2:443) |
// | (1.1.1.1:60005 - 2.2.2.2:80)  | (1.1.1.1:60001 - 2.2.2.2:80)  |
// | (1.1.1.1:60006 - 2.2.2.2:443) | (1.1.1.1:60004 - 2.2.2.2:443) |
// | (2.2.2.2:80 - 1.1.1.1:70000)  | (2.2.2.2:80 - 1.1.1.1:60001)  |
// | (2.2.2.2:90001 - 1.1.1.1:80)  | (2.2.2.2:90001 - 1.1.1.1:80)  |
// | (2.2.2.2:90002 - 1.1.1.1:80)  | (2.2.2.2:90001 - 1.1.1.1:80)  |
// |-------------------------------+-------------------------------|
//
// Note that the ephemeral port numbers just happen to be the number of the
// first "sample" processed (eg. 60001, 60004, 90001) for a given
// (IP-A,IP-B,SERVER_PORT) tuple.
//
// Not strictly necessary, but it may be convenient for the client of this code
// to clear those ephemeral port numbers and have something like (1.1.1.1:0 -
// 2.2.2.2:80) instead of (1.1.1.1:60001 - 2.2.2.2:80) for the "rolled up" keys.
// For that purpose we provide a second method (`ClearEphemeralPort`) that can
// be called once the stream has been processed.
//
// NOTE: this code should not be used in the context of bare-process monitoring.
func (c *ConnectionAggregator) RollupKey(key types.ConnectionKey) types.ConnectionKey {
	if c == nil {
		return key
	}

	// Here we translate the ConnectionKey to a (ipTuple, portTuple) pair.
	// Note that this representation is normalized, so
	//
	// splitKey(IP-A:PORT-A,IP-B:PORT-B) == splitKey(IP-B:PORT-B,IP-A:PORT-A)
	ips, ports, flipped := splitKey(key)

	// First we check if we have seen any traffic from this (IP-A:IP-B) tuple
	portSamples, ok := c.data[ips]
	if !ok {
		// In case we haven't found any existing port tuples we pre-allocate the slice
		portSamples = make([]portTuple, 0, defaultPortSampleSize)
		c.data[ips] = portSamples
	}

	// Next, check if we have seen a "similar" types.ConnectionKey before. In this
	// context "similar" means another type.ConnectionKey that has the same IPs
	// and *at least one port number* matching on the same "side".
	//
	// (IP-A:8888, IP-B:5555) and (IP-A:8888, IP-B:6666) are similar;
	// (IP-A:8888, IP-B:5555) and (IP-A:6666, IP-B:8888) are *not* similar;
	similarPortTuple, ok := c.findSimilar(portSamples, ports)
	if !ok {
		// There are no similar entries, so we store this sample and
		// return the key as it is.
		c.data[ips] = append(c.data[ips], ports)
		return key
	}

	if similarPortTuple.a == ports.a && similarPortTuple.b == ports.b {
		// This is an exact match so we can bail out and return the key as it is
		return key
	}

	if similarPortTuple.serverSide == serverPortUnknown {
		// Determine which side is the server port in case we haven't done it yet.
		// This information is only used when calling `ClearEphemeralPort()`
		if similarPortTuple.a == ports.a {
			similarPortTuple.serverSide = serverPortA
		} else {
			similarPortTuple.serverSide = serverPortB
		}
	}

	// Return the *similar* key we've seen before
	return generateKey(ips, *similarPortTuple, flipped)
}

// ClearEphemeralPort returns a *potentially modified* `types.ConnectionKey`
// with the ephemeral port set to 0. This method is supposed once the whole
// event stream has been consumed and `RollupKey` has been called on every
// connection.
//
// To indicate whether or not the returned key has the ephemeral port cleared,
// we also return a bool value.
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
// Note that the ephemeral port in this case happens to be 60001 simply
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
func (c *ConnectionAggregator) ClearEphemeralPort(key types.ConnectionKey) (types.ConnectionKey, bool) {
	if c == nil {
		return key, false
	}

	ips, ports, flipped := splitKey(key)
	portSamples, ok := c.data[ips]
	if !ok {
		return key, false
	}

	similarPortTuple, ok := c.findSimilar(portSamples, ports)
	if !ok || similarPortTuple.serverSide == serverPortUnknown {
		// We either haven't seen this connection, or were not able to
		// determine the ephemeral port side. In this case we return the key
		// completely unmodified.
		return key, false
	}

	// Create a copy of the port tuple to generate the new key
	ports = *similarPortTuple
	// Clear the client/ephemeral port side
	if ports.serverSide == serverPortA {
		ports.b = 0
	} else {
		ports.a = 0
	}

	return generateKey(ips, ports, flipped), true
}

func (c *ConnectionAggregator) findSimilar(candidates []portTuple, target portTuple) (*portTuple, bool) {
	if len(candidates) == 0 {
		return nil, false
	}

	// NOTE: we're doing a brute force search here because the search space
	// should be quite small. The cardinality of `candidates` is the number of
	// distinct server ports *for a given (IP-A:IP-B) tuple*. In typical
	// containerized workloads this tends to be 1 (a container listens to a
	// single port).  Obviously this is the best case, but I still don't see how
	// this can go, say, beyond dozens of entries, so I don't think we need
	// anything better than this for now.
	for i, portTuple := range candidates {
		if portTuple.a == target.a || portTuple.b == target.b {
			return &candidates[i], true
		}
	}

	return nil, false
}

// normalizeByIP such that srcIP < dstIP so we can index and lookup IPs in a
// deterministic way. In addition to the normalized `types.ConnectionKey` we
// also return a bool indicated whether or not the original tuple was flipped.
func normalizeByIP(key types.ConnectionKey) (normalizedKey types.ConnectionKey, flipped bool) {
	if key.SrcIPHigh > key.DstIPHigh || (key.SrcIPHigh == key.DstIPHigh && key.SrcIPLow > key.DstIPLow) {
		return flipKey(key), true
	}

	return key, false
}

// splitKey maps a `types.ConnectionKey` into a (ipTuple, portTuple) pair.  the
// third return value (bool) indicates whether or not the normalized (ipTuple,
// portTuple), is flipped when compared to the original `key`.
func splitKey(key types.ConnectionKey) (ipTuple, portTuple, bool) {
	normKey, flipped := normalizeByIP(key)

	ips := ipTuple{
		aLow:  normKey.SrcIPLow,
		aHigh: normKey.SrcIPHigh,
		bLow:  normKey.DstIPLow,
		bHigh: normKey.DstIPHigh,
	}

	ports := portTuple{
		a: normKey.SrcPort,
		b: normKey.DstPort,
	}

	return ips, ports, flipped
}

func generateKey(ips ipTuple, ports portTuple, flipped bool) types.ConnectionKey {
	key := types.ConnectionKey{
		SrcIPLow:  ips.aLow,
		SrcIPHigh: ips.aHigh,
		DstIPLow:  ips.bLow,
		DstIPHigh: ips.bHigh,
		SrcPort:   ports.a,
		DstPort:   ports.b,
	}

	if flipped {
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
