// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
)

type kafkaEncoder struct {
	aggregations  map[kafka.KeyTuple]*kafkaAggregationWrapper
	orphanEntries int
}

// aggregationWrapper is meant to handle collision scenarios where multiple
// `ConnectionStats` objects may claim the same `DataStreamsAggregations` object because
// they generate the same kafka.KeyTuple
// TODO: we should probably revist/get rid of this if we ever replace socket
// filters by kprobes, since in that case we would have access to PIDs, and
// could incorporate that information in the `kafka.KeyTuple` struct.
type kafkaAggregationWrapper struct {
	*model.DataStreamsAggregations

	// we keep track of the source and destination ports of the first
	// `ConnectionStats` to claim this `HTTPAggregations` object
	sport, dport uint16
}

func (a *kafkaAggregationWrapper) ValueFor(c network.ConnectionStats) *model.DataStreamsAggregations {
	if a == nil {
		return nil
	}

	if a.sport == 0 && a.dport == 0 {
		// This is the first time a ConnectionStats claim this aggregation. In
		// this case we return the value and save the source and destination
		// ports
		a.sport = c.SPort
		a.dport = c.DPort
		return a.DataStreamsAggregations
	}

	if c.SPort == a.dport && c.DPort == a.sport {
		// We have a collision with another `ConnectionStats`, but this is a
		// legit scenario where we're dealing with the opposite ends of the
		// same connection, which means both server and client are in the same host.
		// In this particular case it is correct to have both connections
		// (client:server and server:client) referencing the same HTTP data.
		return a.DataStreamsAggregations
	}

	// Return nil otherwise. This is to prevent multiple `ConnectionStats` with
	// exactly the same source and destination addresses but different PIDs to
	// "bind" to the same HTTPAggregations object, which would result in a
	// overcount problem. (Note that this is due to the fact that
	// `kafka.KeyTuple` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker proceses share the
	// same socket
	return nil
}

func newKafkaEncoder(payload *network.Connections) *kafkaEncoder {
	if len(payload.Kafka) == 0 {
		return nil
	}

	encoder := &kafkaEncoder{
		aggregations: make(map[kafka.KeyTuple]*kafkaAggregationWrapper, len(payload.Conns)),
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		for _, key := range network.KafkaKeyTuplesFromConn(conn) {
			encoder.aggregations[key] = nil
		}
	}
	encoder.buildAggregations(payload)
	return encoder
}

func (e *kafkaEncoder) GetKafkaAggregations(c network.ConnectionStats) *model.DataStreamsAggregations {
	if e == nil {
		return nil
	}

	keyTuples := network.KafkaKeyTuplesFromConn(c)
	for _, key := range keyTuples {
		if aggregation := e.aggregations[key]; aggregation != nil {
			return e.aggregations[key].ValueFor(c)
		}
	}
	return nil
}

func (e *kafkaEncoder) buildAggregations(payload *network.Connections) {
	fetchAggrSize := make(map[kafka.KeyTuple]int)
	produceAggrSize := make(map[kafka.KeyTuple]int)
	for key, value := range payload.Kafka {
		if value.Data[0] != nil {
			produceAggrSize[key.KeyTuple] += value.Data[0].Count
		}
		if value.Data[1] != nil {
			fetchAggrSize[key.KeyTuple] += value.Data[1].Count
		}
	}

	for key, stats := range payload.Kafka {
		aggregation, ok := e.aggregations[key.KeyTuple]
		if !ok {
			// if there is no matching connection don't even bother to serialize HTTP data
			e.orphanEntries++
			continue
		}

		if aggregation == nil {
			aggregation = &kafkaAggregationWrapper{
				DataStreamsAggregations: &model.DataStreamsAggregations{
					KafkaProduceAggregations: &model.DataStreamsAggregations_KafkaProduceAggregations{
						Stats: make([]*model.DataStreamsAggregations_TopicStats, 0, produceAggrSize[key.KeyTuple]),
					},
					KafkaFetchAggregations: &model.DataStreamsAggregations_KafkaFetchAggregations{
						Stats: make([]*model.DataStreamsAggregations_TopicStats, 0, fetchAggrSize[key.KeyTuple]),
					},
				},
			}
			e.aggregations[key.KeyTuple] = aggregation
		}

		if stats.Data[0] != nil && stats.Data[0].Count > 0 {
			aggregation.KafkaProduceAggregations.Stats = append(aggregation.KafkaProduceAggregations.Stats, &model.DataStreamsAggregations_TopicStats{
				Topic: key.TopicName,
				Count: uint32(stats.Data[0].Count),
			})
		}

		if stats.Data[1] != nil && stats.Data[1].Count > 0 {
			aggregation.KafkaFetchAggregations.Stats = append(aggregation.KafkaFetchAggregations.Stats, &model.DataStreamsAggregations_TopicStats{
				Topic: key.TopicName,
				Count: uint32(stats.Data[1].Count),
			})
		}
	}
}
