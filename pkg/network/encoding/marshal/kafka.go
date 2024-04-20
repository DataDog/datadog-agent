// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"bytes"
	"io"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/types"
)

type kafkaEncoder struct {
	kafkaAggregationsBuilder *model.DataStreamsAggregationsBuilder
	byConnection             *USMConnectionIndex[kafka.Key, *kafka.RequestStat]
}

func newKafkaEncoder(kafkaPayloads map[kafka.Key]*kafka.RequestStat) *kafkaEncoder {
	if len(kafkaPayloads) == 0 {
		return nil
	}

	return &kafkaEncoder{
		kafkaAggregationsBuilder: model.NewDataStreamsAggregationsBuilder(nil),
		byConnection: GroupByConnection("kafka", kafkaPayloads, func(key kafka.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
	}
}

func (e *kafkaEncoder) WriteKafkaAggregations(c network.ConnectionStats, builder *model.ConnectionBuilder) {
	if e == nil {
		return
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return
	}

	builder.SetDataStreamsAggregations(func(b *bytes.Buffer) {
		e.encodeData(connectionData, b)
	})
}

func (e *kafkaEncoder) encodeData(connectionData *USMConnectionData[kafka.Key, *kafka.RequestStat], w io.Writer) {
	e.kafkaAggregationsBuilder.Reset(w)

	for _, kv := range connectionData.Data {
		key := kv.Key
		stats := kv.Value
		e.kafkaAggregationsBuilder.AddKafkaAggregations(func(builder *model.KafkaAggregationBuilder) {
			builder.SetHeader(func(header *model.KafkaRequestHeaderBuilder) {
				header.SetRequest_type(uint32(key.RequestAPIKey))
				header.SetRequest_version(uint32(key.RequestVersion))
			})
			builder.SetTopic(key.TopicName)
			builder.SetCount(uint32(stats.Count))
		})
	}
}

func (e *kafkaEncoder) Close() {
	if e == nil {
		return
	}

	e.byConnection.Close()
}
