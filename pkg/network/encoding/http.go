// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"sync"

	"github.com/gogo/protobuf/proto"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	httpStatsDataPool = sync.Pool{
		New: func() any {
			return new(model.HTTPStats_Data)
		},
	}

	httpStatsPool = sync.Pool{
		New: func() any {
			return new(model.HTTPStats)
		},
	}
)

type httpEncoder struct {
	aggregations   map[types.ConnectionKey]*aggregationWrapper
	staticTags     map[types.ConnectionKey]uint64
	dynamicTagsSet map[types.ConnectionKey]map[string]struct{}

	orphanEntries int
}

// aggregationWrapper is meant to handle collision scenarios where multiple
// `ConnectionStats` objects may claim the same `HTTPAggregations` object because
// they generate the same connection key
// TODO: we should probably revisit/get rid of this if we ever replace socket
// filters by kprobes, since in that case we would have access to PIDs, and
// could incorporate that information in the `types.ConnectionKey` struct.
type aggregationWrapper struct {
	*model.HTTPAggregations

	// we keep track of the source and destination ports of the first
	// `ConnectionStats` to claim this `HTTPAggregations` object
	sport, dport uint16
}

func (a *aggregationWrapper) ValueFor(c network.ConnectionStats) *model.HTTPAggregations {
	if a == nil {
		return nil
	}

	if a.sport == 0 && a.dport == 0 {
		// This is the first time a ConnectionStats claim this aggregation. In
		// this case we return the value and save the source and destination
		// ports
		a.sport = c.SPort
		a.dport = c.DPort
		return a.HTTPAggregations
	}

	if c.SPort == a.dport && c.DPort == a.sport {
		// We have have a collision with another `ConnectionStats`, but this is a
		// legit scenario where we're dealing with the opposite ends of the
		// same connection, which means both server and client are in the same host.
		// In this particular case it is correct to have both connections
		// (client:server and server:client) referencing the same HTTP data.
		return a.HTTPAggregations
	}

	// Return nil otherwise. This is to prevent multiple `ConnectionStats` with
	// exactly the same source and destination addresses but different PIDs to
	// "bind" to the same HTTPAggregations object, which would result in a
	// overcount problem. (Note that this is due to the fact that
	// `types.ConnectionKey` doesn't have a PID field.) This happens mostly in the
	// context of pre-fork web servers, where multiple worker processes share the
	// same socket
	return nil
}

func newHTTPEncoder(payload *network.Connections) *httpEncoder {
	if len(payload.HTTP) == 0 {
		return nil
	}

	encoder := &httpEncoder{
		aggregations:   make(map[types.ConnectionKey]*aggregationWrapper, len(payload.Conns)),
		staticTags:     make(map[types.ConnectionKey]uint64, len(payload.Conns)),
		dynamicTagsSet: make(map[types.ConnectionKey]map[string]struct{}, len(payload.Conns)),
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		for _, key := range network.ConnectionKeysFromConnectionStats(conn) {
			encoder.aggregations[key] = nil
		}
	}

	encoder.buildAggregations(payload)
	return encoder
}

func (e *httpEncoder) GetHTTPAggregationsAndTags(c network.ConnectionStats) (*model.HTTPAggregations, uint64, map[string]struct{}) {
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

func (e *httpEncoder) buildAggregations(payload *network.Connections) {
	aggrSize := make(map[types.ConnectionKey]int, len(payload.HTTP))
	for key := range payload.HTTP {
		aggrSize[key.ConnectionKey]++
	}

	for key, stats := range payload.HTTP {
		aggregation, ok := e.aggregations[key.ConnectionKey]
		if !ok {
			// if there is no matching connection don't even bother to serialize HTTP data
			log.Tracef("Found http orphan connection %v", key.ConnectionKey)
			e.orphanEntries++
			continue
		}

		if aggregation == nil {
			aggregation = &aggregationWrapper{
				HTTPAggregations: &model.HTTPAggregations{
					EndpointAggregations: make([]*model.HTTPStats, 0, aggrSize[key.ConnectionKey]),
				},
			}
			e.aggregations[key.ConnectionKey] = aggregation
		}

		ms := httpStatsPool.Get().(*model.HTTPStats)
		ms.Path = key.Path.Content
		ms.FullPath = key.Path.FullPath
		ms.Method = model.HTTPMethod(key.Method)
		ms.StatsByStatusCode = e.getDataMap(stats.Data)

		staticTags := e.staticTags[key.ConnectionKey]
		var dynamicTags map[string]struct{}
		for status, s := range stats.Data {
			data := ms.StatsByStatusCode[int32(status)]
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

func (e *httpEncoder) getDataMap(stats map[uint16]*http.RequestStat) map[int32]*model.HTTPStats_Data {
	res := make(map[int32]*model.HTTPStats_Data, len(stats))
	for key := range stats {
		res[int32(key)] = httpStatsDataPool.Get().(*model.HTTPStats_Data)
	}
	return res
}

func (e *httpEncoder) Close() {
	if e == nil {
		return
	}
	for _, elem := range e.aggregations {
		if elem == nil {
			continue
		}
		for _, entry := range elem.EndpointAggregations {
			for _, value := range entry.StatsByStatusCode {
				httpStatsDataPool.Put(value)
			}
			httpStatsPool.Put(entry)
		}
	}

	e.aggregations = nil
}
