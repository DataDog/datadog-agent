// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestUSMLookup(t *testing.T) {
	t.Run("without NAT", func(t *testing.T) {
		key := types.NewConnectionKey(
			util.AddressFromString("1.1.1.1"),
			util.AddressFromString("2.2.2.2"),
			60000,
			80,
		)

		// The value doesn't matter for the purposes of this test
		// We only want to make sure that this object is returned during lookups
		val := new(USMConnectionData[struct{}, any])

		data := make(map[types.ConnectionKey]*USMConnectionData[struct{}, any])
		data[key] = val

		// Assert that c1 and c2 (which are symmetrical) "link" to the same aggregation
		c1 := ConnectionStats{
			Source: util.AddressFromString("1.1.1.1"),
			Dest:   util.AddressFromString("2.2.2.2"),
			SPort:  60000,
			DPort:  80,
		}
		assert.Equal(t, val, USMLookup(c1, data))

		c2 := ConnectionStats{
			Source: util.AddressFromString("2.2.2.2"),
			Dest:   util.AddressFromString("1.1.1.1"),
			SPort:  80,
			DPort:  60000,
		}
		assert.Equal(t, val, USMLookup(c2, data))
	})

	t.Run("with NAT", func(t *testing.T) {
		key := types.NewConnectionKey(
			util.AddressFromString("3.3.3.3"),
			util.AddressFromString("4.4.4.4"),
			50000,
			8080,
		)

		val := new(USMConnectionData[struct{}, any])
		data := make(map[types.ConnectionKey]*USMConnectionData[struct{}, any])
		data[key] = val

		// Assert that c1 and c2 (which are symmetrical) "link" to the same aggregation
		c1 := ConnectionStats{
			Source: util.AddressFromString("1.1.1.1"),
			Dest:   util.AddressFromString("2.2.2.2"),
			SPort:  60000,
			DPort:  80,
			IPTranslation: &IPTranslation{
				ReplSrcIP:   util.AddressFromString("3.3.3.3"),
				ReplDstIP:   util.AddressFromString("4.4.4.4"),
				ReplSrcPort: 50000,
				ReplDstPort: 8080,
			},
		}
		assert.Equal(t, val, USMLookup(c1, data))

		c2 := ConnectionStats{
			Source: util.AddressFromString("2.2.2.2"),
			Dest:   util.AddressFromString("1.1.1.1"),
			SPort:  80,
			DPort:  60000,
			IPTranslation: &IPTranslation{
				ReplSrcIP:   util.AddressFromString("4.4.4.4"),
				ReplDstIP:   util.AddressFromString("3.3.3.3"),
				ReplSrcPort: 8080,
				ReplDstPort: 50000,
			},
		}
		assert.Equal(t, val, USMLookup(c2, data))
	})
}

func TestWithKey(t *testing.T) {
	t.Run("without NAT", func(t *testing.T) {
		c := ConnectionStats{
			Source: util.AddressFromString("10.1.1.1"),
			Dest:   util.AddressFromString("10.2.2.2"),
			SPort:  60000,
			DPort:  80,
		}

		shouldGenerateKeys(t, c,
			types.NewConnectionKey(c.Source, c.Dest, c.SPort, c.DPort),
			types.NewConnectionKey(c.Dest, c.Source, c.DPort, c.SPort),
		)
	})

	t.Run("with NAT", func(t *testing.T) {
		c := ConnectionStats{
			Source: util.AddressFromString("10.1.1.1"),
			Dest:   util.AddressFromString("10.2.2.2"),
			SPort:  60000,
			DPort:  80,
			IPTranslation: &IPTranslation{
				ReplSrcIP:   util.AddressFromString("3.3.3.3"),
				ReplDstIP:   util.AddressFromString("4.4.4.4"),
				ReplSrcPort: 50000,
				ReplDstPort: 8080,
			},
		}

		shouldGenerateKeys(t, c,
			types.NewConnectionKey(c.Source, c.Dest, c.SPort, c.DPort),
			types.NewConnectionKey(c.Dest, c.Source, c.DPort, c.SPort),
			types.NewConnectionKey(c.IPTranslation.ReplSrcIP, c.IPTranslation.ReplDstIP, c.IPTranslation.ReplSrcPort, c.IPTranslation.ReplDstPort),
			types.NewConnectionKey(c.IPTranslation.ReplDstIP, c.IPTranslation.ReplSrcIP, c.IPTranslation.ReplDstPort, c.IPTranslation.ReplSrcPort),
		)
	})
}

func shouldGenerateKeys(t *testing.T, c ConnectionStats, expectedKeys ...types.ConnectionKey) {
	var generatedKeys []types.ConnectionKey

	WithKey(c, func(key types.ConnectionKey) bool {
		generatedKeys = append(generatedKeys, key)
		return false
	})

	assert.ElementsMatch(t, expectedKeys, generatedKeys)
}

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
	val1 := http.NewRequestStats(false)
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
	val2 := http.NewRequestStats(false)
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
	val3 := http.NewRequestStats(false)
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
	val4 := http.NewRequestStats(false)
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
	connection1 := ConnectionStats{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("2.2.2.2"),
		SPort:  60000,
		DPort:  80,
	}
	connectionData1 := byConnection.Find(connection1)
	assert.NotNil(t, connectionData1)
	assert.Len(t, connectionData1.Data, 2)
	assert.Condition(t, keyValueExists(connectionData1, key1, val1))
	assert.Condition(t, keyValueExists(connectionData1, key2, val2))

	// Connection 2
	// Assert that (key3, val3) and (key4, val4) were grouped together
	connection2 := ConnectionStats{
		Source: util.AddressFromString("3.3.3.3"),
		Dest:   util.AddressFromString("4.4.4.4"),
		SPort:  60000,
		DPort:  80,
	}
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
