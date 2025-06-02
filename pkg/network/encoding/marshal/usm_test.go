// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestGroupByConnection(t *testing.T) {
	// Connection 1
	key1 := http.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		[]byte("/connection-1-path-1"),
		true,
		http.MethodGet,
	)
	val1 := http.NewRequestStats()
	val1.AddRequest(100, 10.0, 0, nil)

	key2 := http.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		[]byte("/connection-1-path-2"),
		true,
		http.MethodGet,
	)
	val2 := http.NewRequestStats()
	val2.AddRequest(200, 10.0, 0, nil)

	// Connection 2
	key3 := http.NewKey(
		util.AddressFromString("3.3.3.3"),
		util.AddressFromString("4.4.4.4"),
		60000,
		80,
		[]byte("/connection-2-path-1"),
		true,
		http.MethodGet,
	)
	val3 := http.NewRequestStats()
	val3.AddRequest(300, 10.0, 0, nil)

	key4 := http.NewKey(
		util.AddressFromString("3.3.3.3"),
		util.AddressFromString("4.4.4.4"),
		60000,
		80,
		[]byte("/connection-2-path-2"),
		true,
		http.MethodGet,
	)
	val4 := http.NewRequestStats()
	val4.AddRequest(400, 10.0, 0, nil)

	data := map[http.Key]*http.RequestStats{
		key1: val1,
		key2: val2,
		key3: val3,
		key4: val4,
	}

	byConnection := GroupByConnection("http", data, func(httpKey http.Key) types.ConnectionKey {
		return httpKey.ConnectionKey
	})

	// Connection 1
	// Assert that (key1, val1) and (key2, val2) were grouped together
	connection1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("2.2.2.2"),
		SPort:  60000,
		DPort:  80,
	}}
	connectionData1 := byConnection.Find(connection1)
	assert.NotNil(t, connectionData1)
	assert.Len(t, connectionData1.Data, 2)
	assert.Condition(t, keyValueExists(connectionData1, key1, val1))
	assert.Condition(t, keyValueExists(connectionData1, key2, val2))

	// Connection 2
	// Assert that (key3, val3) and (key4, val4) were grouped together
	connection2 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: util.AddressFromString("3.3.3.3"),
		Dest:   util.AddressFromString("4.4.4.4"),
		SPort:  60000,
		DPort:  80,
	}}
	connectionData2 := byConnection.Find(connection2)
	assert.NotNil(t, connectionData2)
	assert.Len(t, connectionData1.Data, 2)
	assert.Condition(t, keyValueExists(connectionData2, key3, val3))
	assert.Condition(t, keyValueExists(connectionData2, key4, val4))
}

func keyValueExists[K, V comparable](connectionData *USMConnectionData[K, V], key K, value V) func() bool {
	return func() bool {
		for _, kv := range connectionData.Data {
			if kv.Key == key && kv.Value == value {
				return true
			}
		}
		return false
	}
}
