// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/DataDog/datadog-agent/pkg/network/kafka"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatKafkaStats(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	kafkaKey1 := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		"topic-1",
	)
	kafkaStats1 := kafka.RequestStats{
		Data: [2]*kafka.RequestStat{
			new(kafka.RequestStat),
			new(kafka.RequestStat),
		},
	}
	kafkaStats1.Data[0].Count = 3 // 3 produces
	kafkaStats1.Data[1].Count = 5 // 5 fetches

	kafkaKey2 := kafkaKey1
	kafkaKey2.TopicName = "topic-2"
	kafkaStats2 := kafka.RequestStats{
		Data: [2]*kafka.RequestStat{
			new(kafka.RequestStat),
			new(kafka.RequestStat),
		},
	}
	kafkaStats2.Data[0].Count = 0 // 3 produces
	kafkaStats2.Data[1].Count = 9 // 5 fetches

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: localhost,
					Dest:   localhost,
					SPort:  clientPort,
					DPort:  serverPort,
				},
			},
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey1: &kafkaStats1,
			kafkaKey2: &kafkaStats2,
		},
	}

	out := &model.DataStreamsAggregations{
		KafkaProduceAggregations: &model.DataStreamsAggregations_KafkaProduceAggregations{
			Stats: []*model.DataStreamsAggregations_TopicStats{
				{
					Topic: "topic-1",
					Count: 3,
				},
			},
		},
		KafkaFetchAggregations: &model.DataStreamsAggregations_KafkaFetchAggregations{
			Stats: []*model.DataStreamsAggregations_TopicStats{
				{
					Topic: "topic-1",
					Count: 5,
				},
				{
					Topic: "topic-2",
					Count: 9,
				},
			},
		},
	}

	kafkaEncoder := newKafkaEncoder(in)
	aggregations := kafkaEncoder.GetKafkaAggregations(in.Conns[0])
	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.KafkaProduceAggregations.Stats, aggregations.KafkaProduceAggregations.Stats)
	assert.ElementsMatch(t, out.KafkaFetchAggregations.Stats, aggregations.KafkaFetchAggregations.Stats)
}

func TestKafkaIDCollisionRegression(t *testing.T) {
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    1,
		},
		{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    2,
		},
	}

	kafkaStats := kafka.RequestStats{
		Data: [2]*kafka.RequestStat{
			{
				Count: 9,
			},
			{
				Count: 2,
			},
		},
	}
	kafkaKey := kafka.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		"topic1",
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey: &kafkaStats,
		},
	}

	kafkaEncoder := newKafkaEncoder(in)

	// assert that the first connection matching the HTTP data will get
	// back a non-nil result
	aggregations := kafkaEncoder.GetKafkaAggregations(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("topic1", aggregations.KafkaProduceAggregations.Stats[0].Topic)
	assert.Equal(uint32(9), aggregations.KafkaProduceAggregations.Stats[0].Count)
	assert.Equal("topic1", aggregations.KafkaFetchAggregations.Stats[0].Topic)
	assert.Equal(uint32(2), aggregations.KafkaFetchAggregations.Stats[0].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the HTTP stats
	// object
	aggregations = kafkaEncoder.GetKafkaAggregations(connections[1])
	assert.Nil(aggregations)
}

func TestKafkaLocalhostScenario(t *testing.T) {
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  80,
			Pid:    1,
		},
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  80,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  60000,
			Pid:    2,
		},
	}

	kafkaStats := kafka.RequestStats{
		Data: [2]*kafka.RequestStat{
			{
				Count: 9,
			},
			{
				Count: 2,
			},
		},
	}

	kafkaKey := kafka.NewKey(
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("127.0.0.1"),
		60000,
		80,
		"topic1",
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey: &kafkaStats,
		},
	}

	kafkaEncoder := newKafkaEncoder(in)

	// assert that both ends (client:server, server:client) of the connection
	// will have Kafka stats
	aggregations := kafkaEncoder.GetKafkaAggregations(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("topic1", aggregations.KafkaProduceAggregations.Stats[0].Topic)
	assert.Equal("topic1", aggregations.KafkaFetchAggregations.Stats[0].Topic)
	assert.Equal(uint32(9), aggregations.KafkaProduceAggregations.Stats[0].Count)
	assert.Equal(uint32(2), aggregations.KafkaFetchAggregations.Stats[0].Count)

	aggregations = kafkaEncoder.GetKafkaAggregations(connections[1])
	assert.NotNil(aggregations)
	assert.Equal("topic1", aggregations.KafkaProduceAggregations.Stats[0].Topic)
	assert.Equal("topic1", aggregations.KafkaFetchAggregations.Stats[0].Topic)
	assert.Equal(uint32(9), aggregations.KafkaProduceAggregations.Stats[0].Count)
	assert.Equal(uint32(2), aggregations.KafkaFetchAggregations.Stats[0].Count)
}
