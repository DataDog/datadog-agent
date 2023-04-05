// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"github.com/gogo/protobuf/proto"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type http2Encoder struct {
	aggregations   map[types.ConnectionKey]*http2AggregationWrapper
	staticTags     map[types.ConnectionKey]uint64
	dynamicTagsSet map[types.ConnectionKey]map[string]struct{}

	orphanEntries int
}

// aggregationWrapper is meant to handle collision scenarios where multiple
// `ConnectionStats` objects may claim the same `HTTP2Aggregations` object because
// they generate the same connection key
// TODO: we should probably revisit/get rid of this if we ever replace socket
// filters by kprobes, since in that case we would have access to PIDs, and
// could incorporate that information in the `types.ConnectionKey` struct.
type http2AggregationWrapper struct {
	*model.HTTP2Aggregations

	// we keep track of the source and destination ports of the first
	// `ConnectionStats` to claim this `HTTP2Aggregations` object
	sport, dport uint16
}

func (e *http2Encoder) GetHTTP2AggregationsAndTags(c network.ConnectionStats) (*model.HTTP2Aggregations, uint64, map[string]struct{}) {
	if e == nil {
		return nil, 0, nil
	}

	connectionKeys := network.ConnectionKeysFromConnectionStats(c)
	for _, key := range connectionKeys {
		if aggregation := e.aggregations[key]; aggregation != nil {
			return e.aggregations[key].ValueFor(c), e.staticTags[key], e.dynamicTagsSet[key]
		}
	}
	return nil, 0, nil
}

func (a *http2AggregationWrapper) ValueFor(c network.ConnectionStats) *model.HTTP2Aggregations {
	if a == nil {
		return nil
	}

	if a.sport == 0 && a.dport == 0 {
		// This is the first time a ConnectionStats claim this aggregation. In
		// this case we return the value and save the source and destination
		// ports
		a.sport = c.SPort
		a.dport = c.DPort
		return a.HTTP2Aggregations
	}

	if c.SPort == a.dport && c.DPort == a.sport {
		// We have have a collision with another `ConnectionStats`, but this is a
		// legit scenario where we're dealing with the opposite ends of the
		// same connection, which means both server and client are in the same host.
		// In this particular case it is correct to have both connections
		// (client:server and server:client) referencing the same HTTP2 data.
		return a.HTTP2Aggregations
	}

	// Return nil otherwise. This is to prevent multiple `ConnectionStats` with
	// exactly the same source and destination addresses but different PIDs to
	// "bind" to the same HTTP2Aggregations object, which would result in a
	// overcount problem. (Note that this is due to the fact that
	// `types.ConnectionKey` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker processes share the
	// same socket
	return nil
}

func newHTTP2Encoder(payload *network.Connections) *http2Encoder {
	if len(payload.HTTP2) == 0 {
		return nil
	}

	encoder := &http2Encoder{
		aggregations:   make(map[types.ConnectionKey]*http2AggregationWrapper, len(payload.Conns)),
		staticTags:     make(map[types.ConnectionKey]uint64, len(payload.Conns)),
		dynamicTagsSet: make(map[types.ConnectionKey]map[string]struct{}, len(payload.Conns)),
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP2 objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		for _, key := range network.ConnectionKeysFromConnectionStats(conn) {
			log.Tracef("Payload has a connection %v and was converted to http2 key %v", conn, key)
			encoder.aggregations[key] = nil
		}
	}

	encoder.buildAggregations(payload)
	return encoder
}

func (e *http2Encoder) buildAggregations(payload *network.Connections) {
	aggrSize := make(map[types.ConnectionKey]int)
	for key := range payload.HTTP2 {
		aggrSize[key.ConnectionKey]++
	}

	for key, stats := range payload.HTTP2 {
		aggregation, ok := e.aggregations[key.ConnectionKey]
		if !ok {
			// if there is no matching connection don't even bother to serialize HTTP2 data
			log.Tracef("Found http2 orphan connection %v", key.ConnectionKey)
			e.orphanEntries++
			continue
		}

		if aggregation == nil {
			aggregation = &http2AggregationWrapper{
				HTTP2Aggregations: &model.HTTP2Aggregations{
					EndpointAggregations: make([]*model.HTTPStats, 0, aggrSize[key.ConnectionKey]),
				},
			}
			e.aggregations[key.ConnectionKey] = aggregation
		}

		ms := &model.HTTPStats{
			Path:              key.Path.Content,
			FullPath:          key.Path.FullPath,
			Method:            model.HTTPMethod(key.Method),
			StatsByStatusCode: make(map[int32]*model.HTTPStats_Data, len(stats.Data)),
		}

		staticTags := e.staticTags[key.ConnectionKey]
		var dynamicTags map[string]struct{}
		for status, s := range stats.Data {
			data, ok := ms.StatsByStatusCode[int32(status)]
			if !ok {
				ms.StatsByStatusCode[int32(status)] = &model.HTTPStats_Data{}
				data = ms.StatsByStatusCode[int32(status)]
			}
			data.Count = uint32(s.Count)

			if latencies := s.Latencies; latencies != nil {
				blob, _ := proto.Marshal(latencies.ToProto())
				data.Latencies = blob
			} else {
				data.FirstLatencySample = s.FirstLatencySample
			}

			staticTags |= s.StaticTags

			// It is a map to aggregate the same tag
			if len(s.DynamicTags) > 0 {
				if dynamicTags == nil {
					dynamicTags = make(map[string]struct{})
				}

				for _, dynamicTag := range s.DynamicTags {
					dynamicTags[dynamicTag] = struct{}{}
				}
			}
		}

		e.staticTags[key.ConnectionKey] = staticTags
		e.dynamicTagsSet[key.ConnectionKey] = dynamicTags

		aggregation.EndpointAggregations = append(aggregation.EndpointAggregations, ms)
	}
}
