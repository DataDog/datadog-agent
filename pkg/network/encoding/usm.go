// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
)

type USMKeyValue struct {
	Key *http.Key
	Value *http.RequestStats
}

type USMGroupedData struct {
	Data []USMKeyValue

	// This is used for handling PID collisions
	sport, dport uint16

	// Used for the purposes of orphan aggregation count
	claimed bool
}

type USMDataByConnection map[types.ConnectionKey]*USMGroupedData

func GroupByConnection(data map[http.Key]*http.RequestStats) USMDataByConnection {
	byConnection := make(USMDataByConnection, len(data)/2)

	for key, value := range data {
		keyCopy := key
		keyVal := USMKeyValue{Key: &keyCopy, Value: value}

		connectionKey := key.ConnectionKey
		connectionData, ok := byConnection[connectionKey]
		if !ok {
			connectionData = new(USMGroupedData)
			byConnection[connectionKey] = connectionData
		}

		connectionData.Data = append(connectionData.Data, keyVal)
	}

	return byConnection
}

func (bc USMDataByConnection) Find(c network.ConnectionStats) *USMGroupedData {
	var connectionData *USMGroupedData

	network.WithKey(c, func (key types.ConnectionKey) (stopIteration bool) {
		val, ok := bc[key]
		if !ok {
			return false
		}

		connectionData = val
		connectionData.claimed = true
		return true
	})

	return connectionData
}

func (bc USMDataByConnection) OrphanAggregationCount() int {
	var total int
	for _, value := range bc {
		if !value.claimed {
			total += len(value.Data)
		}
	}
	return total
}

func (gd *USMGroupedData) IsPIDCollision(c network.ConnectionStats) bool {
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
	// "bind" to the same HTTPAggregations object, which would result in a
	// overcount problem. (Note that this is due to the fact that
	// `types.ConnectionKey` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker processes share the
	// same socket
	return true
}
