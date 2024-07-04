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
)

type kafkaEncoder struct {
	kafkaAggregationsBuilder *model.DataStreamsAggregationsBuilder
}

func newKafkaEncoder() *kafkaEncoder {
	return &kafkaEncoder{
		kafkaAggregationsBuilder: model.NewDataStreamsAggregationsBuilder(nil),
	}
}

func (e *kafkaEncoder) WriteKafkaAggregations(c network.ConnectionStats, builder *model.ConnectionBuilder) uint64 {
	if e == nil || len(c.KafkaStats) == 0 {
		return 0
	}

	staticTags := uint64(0)
	builder.SetDataStreamsAggregations(func(b *bytes.Buffer) {
		staticTags = e.encodeData(c.KafkaStats, b)
	})

	return staticTags
}

func (e *kafkaEncoder) encodeData(connectionData []network.USMKeyValue[kafka.Key, *kafka.RequestStat], w io.Writer) uint64 {
	var staticTags uint64
	e.kafkaAggregationsBuilder.Reset(w)

	for _, kv := range connectionData {
		key := kv.Key
		stats := kv.Value
		staticTags |= stats.StaticTags
		e.kafkaAggregationsBuilder.AddKafkaAggregations(func(builder *model.KafkaAggregationBuilder) {
			builder.SetHeader(func(header *model.KafkaRequestHeaderBuilder) {
				header.SetRequest_type(uint32(key.RequestAPIKey))
				header.SetRequest_version(uint32(key.RequestVersion))
			})
			builder.SetTopic(key.TopicName)
			builder.SetCount(uint32(stats.Count))
		})
	}
	return staticTags
}

func (e *kafkaEncoder) Close() {}
