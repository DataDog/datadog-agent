// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// USMDataIndexes provides a generic container where data is indexed by
// Connection, so it can be later on used during the encoding process
type USMDataByConnection[K comparable, V any] struct {
	data     map[types.ConnectionKey]*USMConnectionData[K, V]
	protocol string
	once     sync.Once
}

// USMConnectionData aggregates USM data belonging to a specific connection
type USMConnectionData[K comparable, V any] struct {
	Data []USMKeyValue[K, V]

	// This is used for handling PID collisions
	// See notes in `IsPIDCollision`
	sport, dport uint16

	// Used for the purposes of orphan aggregation count
	claimed bool

	// Used during the first pass to determine the size of the `Data`
	// dataSize int
}

type USMKeyValue[K comparable, V any] struct {
	Key   *K
	Value V
}

func GroupByConnection[K comparable, V any](protocol string, data map[K]V, keyGen func(K) types.ConnectionKey) *USMDataByConnection[K, V] {
	byConnection := &USMDataByConnection[K, V]{
		data: make(map[types.ConnectionKey]*USMConnectionData[K, V], len(data)/2),
	}

	for key, value := range data {
		keyCopy := key
		keyVal := USMKeyValue[K, V]{Key: &keyCopy, Value: value}

		connectionKey := keyGen(key)
		connectionData, ok := byConnection.data[connectionKey]
		if !ok {
			connectionData = new(USMConnectionData[K, V])
			byConnection.data[connectionKey] = connectionData
		}

		connectionData.Data = append(connectionData.Data, keyVal)
	}

	return byConnection
}

func (bc *USMDataByConnection[K, V]) Find(c network.ConnectionStats) *USMConnectionData[K, V] {
	var connectionData *USMConnectionData[K, V]
	network.WithKey(c, func(key types.ConnectionKey) (stopIteration bool) {
		val, ok := bc.data[key]
		if !ok {
			return false
		}

		connectionData = val
		connectionData.claimed = true
		return true
	})

	return connectionData
}

func (gd *USMConnectionData[K, V]) IsPIDCollision(c network.ConnectionStats) bool {
	if gd.sport == 0 && gd.dport == 0 {
		// This is the first time a ConnectionStats claim this data. In this
		// case we return the value and save the source and destination ports
		gd.sport = c.SPort
		gd.dport = c.DPort
		return false
	}

	if c.SPort == gd.dport && c.DPort == gd.sport {
		// We have have a collision with another `ConnectionStats`, but this is a
		// legit scenario where we're dealing with the opposite ends of the
		// same connection, which means both server and client are in the same host.
		// In this particular case it is correct to have both connections
		// (client:server and server:client) referencing the same HTTP data.
		return false
	}

	// Return true otherwise. This is to prevent multiple `ConnectionStats` with
	// exactly the same source and destination addresses but different PIDs to
	// "bind" to the same USM aggregation object, which would result in a
	// overcount problem. (Note that this is due to the fact that
	// `types.ConnectionKey` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker processes share the
	// same socket
	return true
}

func (bc *USMDataByConnection[K, V]) Close() {
	bc.once.Do(func() {
		// Determine count of orphan aggregations
		var total int
		for _, value := range bc.data {
			if !value.claimed {
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

		telemetry.NewMetric(
			fmt.Sprintf("usm.%s.orphan_aggregations", bc.protocol),
			telemetry.OptMonotonic,
			telemetry.OptExpvar,
			telemetry.OptStatsd,
		).Add(int64(total))
	})
}
