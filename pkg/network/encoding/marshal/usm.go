// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"fmt"
	"sync"

	"github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// USMConnectionIndex provides a generic container for USM data pre-aggregated by connection
type USMConnectionIndex[K comparable, V any] struct {
	lookupFn func(network.ConnectionStats, map[types.ConnectionKey]*USMConnectionData[K, V]) *USMConnectionData[K, V]
	data     map[types.ConnectionKey]*USMConnectionData[K, V]
	protocol string
	once     sync.Once

	// sourced from the agent config
	enableConnectionRollup bool
}

// USMConnectionData aggregates all USM data associated to a specific connection
type USMConnectionData[K comparable, V any] struct {
	Data []USMKeyValue[K, V]

	// This is used for handling PID collisions
	// See notes in `IsPIDCollision`
	sport, dport uint16

	// Used for the purposes of orphan aggregation count
	claimed bool
}

// USMKeyValue is a generic container for USM data
type USMKeyValue[K comparable, V any] struct {
	Key   K
	Value V
}

// GroupByConnection generates a `USMConnectionIndex` from a generic `map[K]V` data structure.
// In addition to the `data` argument the caller must provide a `keyGen` function that
// essentially translates `K` to a `types.ConnectionKey` and a `protocol` name.
func GroupByConnection[K comparable, V any](protocol string, data map[K]V, keyGen func(K) types.ConnectionKey) *USMConnectionIndex[K, V] {
	byConnection := &USMConnectionIndex[K, V]{
		protocol: protocol,
		lookupFn: USMLookup[K, V],

		// Experimental: Connection Rollups
		enableConnectionRollup: config.SystemProbe.GetBool("service_monitoring_config.enable_connection_rollup"),
	}

	// The map intended to calculate how many entries we actually need in byConnection.data, and for each entry
	// how many elements it has in it.
	entriesSizeMap := make(map[types.ConnectionKey]int)

	// In the first pass we instantiate the map and calculate the number of
	// USM aggregation objects per connection
	for key := range data {
		entriesSizeMap[keyGen(key)]++
	}

	byConnection.data = make(map[types.ConnectionKey]*USMConnectionData[K, V], len(entriesSizeMap))

	// In the second pass we create a slice for each `USMConnectionData` entry
	// in the map using the pre-determined sizes from the previous iteration and
	// append the USM aggregation objects to it
	for key, value := range data {
		connectionKey := keyGen(key)
		connectionData, ok := byConnection.data[connectionKey]
		if !ok {
			connectionData = new(USMConnectionData[K, V])
			connectionData.Data = make([]USMKeyValue[K, V], 0, entriesSizeMap[keyGen(key)])
			byConnection.data[connectionKey] = connectionData
		}

		connectionData.Data = append(connectionData.Data, USMKeyValue[K, V]{
			Key:   key,
			Value: value,
		})
	}

	return byConnection
}

// Find returns a `USMConnectionData` object associated to given `network.ConnectionStats`
// The returned object will include all USM aggregation associated to this connection
func (bc *USMConnectionIndex[K, V]) Find(c network.ConnectionStats) *USMConnectionData[K, V] {
	// Early return because USM currently doesn't support any protocol over UDP
	if c.Type != network.TCP {
		return nil
	}

	result := bc.find(c)
	if result != nil {
		// Mark `USMConnectionData` as claimed for the purposes of orphan
		// aggregation reporting
		result.claimed = true
	}

	if log.ShouldLog(seelog.TraceLvl) {
		log.Tracef("could not find connection %+v in usm data", c)
	}

	return result
}

func (bc *USMConnectionIndex[K, V]) find(c network.ConnectionStats) *USMConnectionData[K, V] {
	result := bc.lookupFn(c, bc.data)
	if result != nil || !bc.enableConnectionRollup {
		return result
	}

	// If no matches were found and we have connection rollups enabled, perfom
	// 2 additional lookups with each port set to 0
	tmp := c.SPort
	c.SPort = 0
	result = bc.lookupFn(c, bc.data)
	if result != nil {
		return result
	}

	c.SPort = tmp
	c.DPort = 0
	return bc.lookupFn(c, bc.data)
}

// IsPIDCollision can be called on each lookup result returned by
// `USMConnectionIndex.Find`. This is intended to avoid over-reporting USM stats
// in the context of PID "collisions". For example, let's say you have the
// following two connections:
//
// Connection 1: srcA, dstB, pid X
// Connection 2: srcA, dstB, pid Y
//
// And some USM data that is associated to: srcA, dstB (note that data from socket
// filter programs doesn't include PIDs)
//
// The purpose of this check is to avoid letting `Connection 1` and `Connection 2`
// be associated to the same USM aggregation object.
//
// So whichever connection "claims" the aggregation first will return `false`
// for `IsPIDCollision`, and any other following connection calling this method
// will get a `true` return value back.
//
// Notice that this PID collision scenario is typical in the context pre-forked
// webservers such as NGINX, where multiple worker processes will share the same
// listen socket.
func (gd *USMConnectionData[K, V]) IsPIDCollision(c network.ConnectionStats) bool {
	if gd.sport == 0 && gd.dport == 0 {
		// This is the first time a ConnectionStats claim this data. In this
		// case we return the value and save the source and destination ports
		gd.sport = c.SPort
		gd.dport = c.DPort
		return false
	}

	if c.SPort == gd.dport && c.DPort == gd.sport {
		// We have a collision with another `ConnectionStats`, but this is a
		// legit scenario where we're dealing with the opposite ends of the
		// same connection, which means both server and client are in the same host.
		// In this particular case it is correct to have both connections
		// (client:server and server:client) referencing the same HTTP data.
		return false
	}

	// Return true otherwise. This is to prevent multiple `ConnectionStats` with
	// exactly the same source and destination addresses but different PIDs to
	// "bind" to the same USM aggregation object, which would result in an
	// overcount problem. (Note that this is due to the fact that
	// `types.ConnectionKey` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker processes share the
	// same socket
	return true
}

// Close `USMConnectionIndex` and report orphan aggregations
func (bc *USMConnectionIndex[K, V]) Close() {
	bc.once.Do(func() {
		// Count the number of USM connections for this particular protocol
		telemetry.NewCounter(
			fmt.Sprintf("usm.%s.connections", bc.protocol),
			telemetry.OptPrometheus,
		).Add(int64(len(bc.data)))

		// Determine count of orphan aggregations
		var total int
		for key, value := range bc.data {
			if !value.claimed {
				if log.ShouldLog(seelog.TraceLvl) {
					log.Tracef("key %+v unclaimed", key)
				}
				total += len(value.Data)
			}
		}

		if total == 0 {
			return
		}

		log.Debugf(
			"detected orphan %s aggregations. this may be caused by conntrack sampling or missed tcp close events. count=%d",
			bc.protocol,
			total,
		)

		telemetry.NewCounter(
			fmt.Sprintf("usm.%s.orphan_aggregations", bc.protocol),
			telemetry.OptStatsd,
		).Add(int64(total))
	})
}
