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

	httpStatusCodeMap = sync.Pool{
		New: func() any {
			m := make(map[int32]*model.HTTPStats_Data)
			return &m
		},
	}
)

type httpEncoder struct {
	byConnection USMDataByConnection

	// cached object
	aggregations *model.HTTPAggregations

	// list of *pointers* to maps so they can be returned to the pool
	toRelease []*map[int32]*model.HTTPStats_Data
}


func newHTTPEncoder(payload *network.Connections) *httpEncoder {
	if len(payload.HTTP) == 0 {
		return nil
	}

	return &httpEncoder{
		byConnection: GroupByConnection(payload.HTTP),
		aggregations: new(model.HTTPAggregations),
	}
}

func (e *httpEncoder) GetHTTPAggregationsAndTags(c network.ConnectionStats) ([]byte, uint64, map[string]struct{}) {
	if e == nil {
		return nil, 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return nil, 0, nil
	}

	return e.encodeData(connectionData)
}

func (e *httpEncoder) encodeData(connectionData *USMGroupedData) ([]byte, uint64, map[string]struct{}) {
	e.reset()

	var staticTags uint64
	var dynamicTags map[string]struct{}

	for _, kvPair := range connectionData.Data {
		key := kvPair.Key
		stats := kvPair.Value

		ms := httpStatsPool.Get().(*model.HTTPStats)
		ms.Path = key.Path.Content
		ms.FullPath = key.Path.FullPath
		ms.Method = model.HTTPMethod(key.Method)
		ms.StatsByStatusCode = e.getDataMap(stats.Data)


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
			for _, dynamicTag := range s.DynamicTags {
				dynamicTags[dynamicTag] = struct{}{}
			}
		}

		e.aggregations.EndpointAggregations = append(e.aggregations.EndpointAggregations, ms)
	}

	serializedData, _ := proto.Marshal(e.aggregations)
	return serializedData, staticTags, dynamicTags
}

// TODO: improve this so the caller doesn't need to save the value
func (e *httpEncoder) OrphanAggregations() int {
	if e == nil {
		return 0
	}

	return e.byConnection.OrphanAggregationCount()
}

func (e *httpEncoder) getDataMap(stats map[uint16]*http.RequestStat) map[int32]*model.HTTPStats_Data {
	resPtr := httpStatusCodeMap.Get().(*map[int32]*model.HTTPStats_Data)
	e.toRelease = append(e.toRelease, resPtr)

	res := *resPtr
	for key := range stats {
		res[int32(key)] = httpStatsDataPool.Get().(*model.HTTPStats_Data)
	}
	return res
}

func (e *httpEncoder) reset() {
	if e == nil {
		return
	}

	for i, endpointAggregation := range e.aggregations.EndpointAggregations {
		for _, s := range endpointAggregation.StatsByStatusCode {
			s.Reset()
			httpStatsDataPool.Put(s)
		}

		// this is an idiom recognized by the go compiler and does not
		// result in iterating in the map, but clearing it
		// TODO: add link to source
		for k := range endpointAggregation.StatsByStatusCode {
			delete(endpointAggregation.StatsByStatusCode, k)
		}

		endpointAggregation.Reset()
		httpStatsPool.Put(endpointAggregation)
		e.aggregations.EndpointAggregations[i] = nil
	}

	for i, mapPtr := range e.toRelease {
		httpStatusCodeMap.Put(mapPtr)
		e.toRelease[i] = nil
	}
	e.toRelease = e.toRelease[:0]
	e.aggregations.EndpointAggregations = e.aggregations.EndpointAggregations[:0]
}
