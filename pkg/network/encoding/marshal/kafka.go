// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

import (
	"sync"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
)

var kafkaAggregationPool = sync.Pool{
	New: func() any {
		return &model.KafkaAggregation{
			Header: new(model.KafkaRequestHeader),
		}
	},
}

type kafkaEncoder struct {
	// cached object
	aggregations *model.DataStreamsAggregations
}

func newKafkaEncoder() *kafkaEncoder {
	return &kafkaEncoder{
		aggregations: &model.DataStreamsAggregations{
			// It's not that important to get the initial size of this slice
			// right because we're re-using it multiple times and should quickly
			// converge to a final size after a few calls to
			// `GetKafkaAggregations`
			KafkaAggregations: make([]*model.KafkaAggregation, 0, 10),
		},
	}
}

func (e *kafkaEncoder) GetKafkaAggregations(c network.ConnectionStats) []byte {
	if len(c.KafkaStats) == 0 {
		return nil
	}

	return e.encodeData(c.KafkaStats)
}

func (e *kafkaEncoder) Close() {
	if e == nil {
		return
	}

	e.reset()
}

func (e *kafkaEncoder) encodeData(connectionData []network.USMKeyValue[kafka.Key, *kafka.RequestStat]) []byte {
	e.reset()

	for _, kv := range connectionData {
		key := kv.Key
		stats := kv.Value

		kafkaAggregation := kafkaAggregationPool.Get().(*model.KafkaAggregation)

		kafkaAggregation.Header.RequestType = uint32(key.RequestAPIKey)
		kafkaAggregation.Header.RequestVersion = uint32(key.RequestVersion)
		kafkaAggregation.Topic = key.TopicName
		kafkaAggregation.Count = uint32(stats.Count)

		e.aggregations.KafkaAggregations = append(e.aggregations.KafkaAggregations, kafkaAggregation)
	}

	serializedData, _ := proto.Marshal(e.aggregations)
	return serializedData
}

func (e *kafkaEncoder) reset() {
	if e == nil {
		return
	}

	for i, kafkaAggregation := range e.aggregations.KafkaAggregations {
		// The pooled *model.KafkaAggregation object comes along with a
		// pre-allocated *model.KafkaHeader object as well, so we ensure that we
		// clean both objects but keep them linked together before returning it
		// to the pool.

		header := kafkaAggregation.Header
		header.Reset()

		kafkaAggregation.Reset()
		kafkaAggregation.Header = header

		kafkaAggregationPool.Put(kafkaAggregation)
		e.aggregations.KafkaAggregations[i] = nil
	}

	e.aggregations.KafkaAggregations = e.aggregations.KafkaAggregations[:0]
}
