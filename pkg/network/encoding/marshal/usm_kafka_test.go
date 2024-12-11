// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func skipIfNotLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the feature is only supported on linux.")
	}
}

const (
	clientPort  = uint16(1234)
	serverPort  = uint16(12345)
	topicName   = "TopicName"
	apiVersion1 = 1
	apiVersion2 = 1
)

var (
	localhost         = util.AddressFromString("127.0.0.1")
	defaultConnection = network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Source: localhost,
		Dest:   localhost,
		SPort:  clientPort,
		DPort:  serverPort,
	}}
)

type KafkaSuite struct {
	suite.Suite
}

func TestKafkaStats(t *testing.T) {
	skipIfNotLinux(t)
	suite.Run(t, &KafkaSuite{})
}

func (s *KafkaSuite) TestFormatKafkaStats() {
	t := s.T()

	kafkaKey1 := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		topicName,
		kafka.ProduceAPIKey,
		apiVersion1,
	)
	kafkaKey2 := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		topicName,
		kafka.FetchAPIKey,
		apiVersion2,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				defaultConnection,
			},
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey1: {
				ErrorCodeToStat: map[int32]*kafka.RequestStat{
					0: {Count: 10},
					1: {Count: 2},
				},
			},
			kafkaKey2: {
				ErrorCodeToStat: map[int32]*kafka.RequestStat{
					0:  {Count: 2},
					10: {Count: 5},
				},
			},
		},
	}
	out := &model.DataStreamsAggregations{
		KafkaAggregations: []*model.KafkaAggregation{
			{
				Header: &model.KafkaRequestHeader{
					RequestType:    kafka.ProduceAPIKey,
					RequestVersion: apiVersion1,
				},
				Topic:            "TopicName",
				StatsByErrorCode: map[int32]*model.KafkaStats{0: {Count: 10}, 1: {Count: 2}},
			},
			{
				Header: &model.KafkaRequestHeader{
					RequestType:    kafka.FetchAPIKey,
					RequestVersion: apiVersion2,
				},
				Topic:            "TopicName",
				StatsByErrorCode: map[int32]*model.KafkaStats{0: {Count: 2}, 10: {Count: 5}},
			},
		},
	}

	encoder := newKafkaEncoder(in.Kafka)
	t.Cleanup(encoder.Close)

	aggregations := getKafkaAggregations(t, encoder, in.Conns[0])

	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.KafkaAggregations, aggregations.KafkaAggregations)
}

func (s *KafkaSuite) TestKafkaIDCollisionRegression() {
	t := s.T()
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  clientPort,
			Dest:   localhost,
			DPort:  serverPort,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  clientPort,
			Dest:   localhost,
			DPort:  serverPort,
			Pid:    2,
		}},
	}

	kafkaKey := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		topicName,
		kafka.ProduceAPIKey,
		apiVersion1,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey: {
				ErrorCodeToStat: map[int32]*kafka.RequestStat{
					0: {Count: 10},
				},
			},
		},
	}

	encoder := newKafkaEncoder(in.Kafka)
	t.Cleanup(encoder.Close)
	aggregations := getKafkaAggregations(t, encoder, in.Conns[0])

	// assert that the first connection matching the Kafka data will get back a non-nil result
	assert.Equal(topicName, aggregations.KafkaAggregations[0].Topic)
	assert.Equal(uint32(10), aggregations.KafkaAggregations[0].StatsByErrorCode[0].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the Kafka stats
	// object
	streamer := NewProtoTestStreamer[*model.Connection]()
	encoder.WriteKafkaAggregations(in.Conns[1], model.NewConnectionBuilder(streamer))
	var conn model.Connection
	streamer.Unwrap(t, &conn)
	assert.Empty(conn.DataStreamsAggregations)
}

func (s *KafkaSuite) TestKafkaLocalhostScenario() {
	t := s.T()
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  clientPort,
			Dest:   localhost,
			DPort:  serverPort,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  serverPort,
			Dest:   localhost,
			DPort:  clientPort,
			Pid:    2,
		}},
	}

	kafkaKey := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		topicName,
		kafka.FetchAPIKey,
		apiVersion2,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey: {
				ErrorCodeToStat: map[int32]*kafka.RequestStat{
					0: {Count: 10},
				},
			},
		},
	}

	encoder := newKafkaEncoder(in.Kafka)
	t.Cleanup(encoder.Close)

	// assert that both ends (client:server, server:client) of the connection
	// will have Kafka stats
	for _, conn := range in.Conns {
		aggregations := getKafkaAggregations(t, encoder, conn)
		assert.Equal(topicName, aggregations.KafkaAggregations[0].Topic)
		assert.Equal(uint32(10), aggregations.KafkaAggregations[0].StatsByErrorCode[0].Count)
	}
}

func getKafkaAggregations(t *testing.T, encoder *kafkaEncoder, c network.ConnectionStats) *model.DataStreamsAggregations {
	streamer := NewProtoTestStreamer[*model.Connection]()
	encoder.WriteKafkaAggregations(c, model.NewConnectionBuilder(streamer))

	var conn model.Connection
	streamer.Unwrap(t, &conn)

	var aggregations model.DataStreamsAggregations
	err := proto.Unmarshal(conn.DataStreamsAggregations, &aggregations)
	require.NoError(t, err)

	return &aggregations
}

func generateBenchMarkPayloadKafka(entries uint16) network.Connections {
	localhost := util.AddressFromString("127.0.0.1")

	payload := network.Connections{
		BufferedData: network.BufferedData{
			Conns: make([]network.ConnectionStats, 1),
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{},
	}

	payload.Conns[0].Dest = localhost
	payload.Conns[0].Source = localhost
	payload.Conns[0].DPort = 1111
	payload.Conns[0].SPort = 1112

	for index := uint16(0); index < entries; index++ {
		payload.Kafka[kafka.NewKey(
			localhost,
			localhost,
			1112,
			1111,
			fmt.Sprintf("%s-%d", topicName, index+1),
			kafka.ProduceAPIKey,
			apiVersion1,
		)] = &kafka.RequestStats{
			ErrorCodeToStat: map[int32]*kafka.RequestStat{0: {Count: 10}},
		}
	}

	return payload
}

func commonBenchmarkKafkaEncoder(b *testing.B, entries uint16) {
	payload := generateBenchMarkPayloadKafka(entries)
	streamer := NewProtoTestStreamer[*model.Connection]()
	a := model.NewConnectionBuilder(streamer)
	b.ResetTimer()
	b.ReportAllocs()
	var h *kafkaEncoder
	for i := 0; i < b.N; i++ {
		h = newKafkaEncoder(payload.Kafka)
		streamer.Reset()
		h.WriteKafkaAggregations(payload.Conns[0], a)
		h.Close()
	}
}

func BenchmarkKafkaEncoder100Requests(b *testing.B) {
	commonBenchmarkKafkaEncoder(b, 100)
}

func BenchmarkKafkaEncoder10000Requests(b *testing.B) {
	commonBenchmarkKafkaEncoder(b, 10000)
}
