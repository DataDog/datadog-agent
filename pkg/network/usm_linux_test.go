// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package network

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/redis"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestHTTP2Stats(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	getStats := func(path string) map[protocols.ProtocolType]interface{} {
		key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, []byte(path), true, http.MethodGet)

		http2Stats := make(map[http.Key]*http.RequestStats)
		http2Stats[key] = http.NewRequestStats()

		usmStats := make(map[protocols.ProtocolType]interface{})
		usmStats[protocols.HTTP2] = http2Stats

		return usmStats
	}

	// Register client & pass in HTTP2 stats
	state := newDefaultState()
	delta := state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, getStats("/testpath"))

	// Verify connection has HTTP2 data embedded in it
	assert.Len(t, delta.USMData.HTTP2, 1)

	// Verify HTTP2 data has been flushed
	delta = state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.HTTP2, 0)
}

func TestHTTP2StatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	getStats := func(path string) map[protocols.ProtocolType]interface{} {
		http2Stats := make(map[http.Key]*http.RequestStats)
		key := http.NewKey(c.Source, c.Dest, c.SPort, c.DPort, []byte(path), true, http.MethodGet)
		http2Stats[key] = http.NewRequestStats()

		usmStats := make(map[protocols.ProtocolType]interface{})
		usmStats[protocols.HTTP2] = http2Stats

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
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).USMData.HTTP2, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).USMData.HTTP2, 0)

	// Store the connection to both clients & pass HTTP2 stats to the first client
	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	delta := state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath"))
	assert.Len(t, delta.USMData.HTTP2, 1)

	// Verify that the HTTP2 stats were also stored in the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.HTTP2, 1)

	// Register a third client & verify that it does not have the HTTP2 stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.HTTP2, 0)

	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	// Pass in new HTTP2 stats to the first client
	delta = state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("/testpath2"))
	assert.Len(t, delta.USMData.HTTP2, 1)

	// And the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, getStats("/testpath3"))
	assert.Len(t, delta.USMData.HTTP2, 2)

	// Verify that the third client also accumulated both new HTTP2 stats
	delta = state.GetDelta(client3, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.HTTP2, 2)
}

func TestRedisStats(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	key := redis.NewKey(c.Source, c.Dest, c.SPort, c.DPort, redis.GetCommand, "test-key", false)

	redisStats := make(map[redis.Key]*redis.RequestStats)
	redisStats[key] = &redis.RequestStats{
		ErrorToStats: map[bool]*redis.RequestStat{
			false: {Count: 2},
		},
	}
	usmStats := make(map[protocols.ProtocolType]interface{})
	usmStats[protocols.Redis] = redisStats

	// Register client & pass in Redis stats
	state := newDefaultState()
	delta := state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, usmStats)

	// Verify connection has Redis data embedded in it
	assert.Len(t, delta.USMData.Redis, 1)

	// Verify Redis data has been flushed
	delta = state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.Redis, 0)
}

func TestRedisStatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	getStats := func(keyName string) map[protocols.ProtocolType]interface{} {
		redisStats := make(map[redis.Key]*redis.RequestStats)
		key := redis.NewKey(c.Source, c.Dest, c.SPort, c.DPort, redis.GetCommand, keyName, false)
		redisStats[key] = &redis.RequestStats{
			ErrorToStats: map[bool]*redis.RequestStat{
				false: {Count: 2},
			},
		}

		usmStats := make(map[protocols.ProtocolType]interface{})
		usmStats[protocols.Redis] = redisStats

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
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).USMData.Redis, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).USMData.Redis, 0)

	// Store the connection to both clients & pass Redis stats to the first client
	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	delta := state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("key-name"))
	assert.Len(t, delta.USMData.Redis, 1)

	// Verify that the Redis stats were also stored in the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.Redis, 1)

	// Register a third client & verify that it does not have the Redis stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.Redis, 0)

	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	// Pass in new Redis stats to the first client
	delta = state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("key-name"))
	assert.Len(t, delta.USMData.Redis, 1)

	// And the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, getStats("key-name-2"))
	assert.Len(t, delta.USMData.Redis, 2)

	// Verify that the third client also accumulated both Redis stats
	delta = state.GetDelta(client3, latestEpochTime(), nil, nil, getStats("key-name-2"))
	assert.Len(t, delta.USMData.Redis, 2)
}

func TestKafkaStats(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	key := kafka.NewKey(c.Source, c.Dest, c.SPort, c.DPort, "my-topic", kafka.ProduceAPIKey, 1)

	kafkaStats := make(map[kafka.Key]*kafka.RequestStats)
	kafkaStats[key] = &kafka.RequestStats{
		ErrorCodeToStat: map[int32]*kafka.RequestStat{
			0: {Count: 2},
		},
	}
	usmStats := make(map[protocols.ProtocolType]interface{})
	usmStats[protocols.Kafka] = kafkaStats

	// Register client & pass in Kafka stats
	state := newDefaultState()
	delta := state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, usmStats)

	// Verify connection has Kafka data embedded in it
	assert.Len(t, delta.USMData.Kafka, 1)

	// Verify Kafka data has been flushed
	delta = state.GetDelta("client", latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.Kafka, 0)
}

func TestKafkaStatsWithMultipleClients(t *testing.T) {
	c := ConnectionStats{ConnectionTuple: ConnectionTuple{
		Source: util.AddressFromString("1.1.1.1"),
		Dest:   util.AddressFromString("0.0.0.0"),
		SPort:  1000,
		DPort:  80,
	}}

	getStats := func(topicName string) map[protocols.ProtocolType]interface{} {
		kafkaStats := make(map[kafka.Key]*kafka.RequestStats)
		key := kafka.NewKey(c.Source, c.Dest, c.SPort, c.DPort, topicName, kafka.ProduceAPIKey, 1)
		kafkaStats[key] = &kafka.RequestStats{
			ErrorCodeToStat: map[int32]*kafka.RequestStat{
				0: {Count: 2},
			},
		}

		usmStats := make(map[protocols.ProtocolType]interface{})
		usmStats[protocols.Kafka] = kafkaStats

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
	assert.Len(t, state.GetDelta(client1, latestEpochTime(), nil, nil, nil).USMData.Kafka, 0)
	assert.Len(t, state.GetDelta(client2, latestEpochTime(), nil, nil, nil).USMData.Kafka, 0)

	// Store the connection to both clients & pass HTTP stats to the first client
	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	delta := state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("my-topic"))
	assert.Len(t, delta.USMData.Kafka, 1)

	// Verify that the HTTP stats were also stored in the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, nil)
	assert.Len(t, delta.USMData.Kafka, 1)

	// Register a third client & verify that it does not have the Kafka stats
	delta = state.GetDelta(client3, latestEpochTime(), []ConnectionStats{c}, nil, nil)
	assert.Len(t, delta.USMData.Kafka, 0)

	c.LastUpdateEpoch = latestEpochTime()
	state.StoreClosedConnection(&c)

	// Pass in new Kafka stats to the first client
	delta = state.GetDelta(client1, latestEpochTime(), nil, nil, getStats("my-topic"))
	assert.Len(t, delta.USMData.Kafka, 1)

	// And the second client
	delta = state.GetDelta(client2, latestEpochTime(), nil, nil, getStats("my-topic2"))
	assert.Len(t, delta.USMData.Kafka, 2)

	// Verify that the third client also accumulated both Kafka stats
	delta = state.GetDelta(client3, latestEpochTime(), nil, nil, getStats("my-topic2"))
	assert.Len(t, delta.USMData.Kafka, 2)
}
