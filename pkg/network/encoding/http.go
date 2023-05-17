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
	byConnection *USMConnectionIndex[http.Key, *http.RequestStats]

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
		byConnection: GroupByConnection("http", payload.HTTP, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
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

func (e *httpEncoder) encodeData(connectionData *USMConnectionData[http.Key, *http.RequestStats]) ([]byte, uint64, map[string]struct{}) {
	e.reset()

	var staticTags uint64
	dynamicTags := make(map[string]struct{})

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

func (e *httpEncoder) Close() {
	if e == nil {
		return
	}

	e.reset()
	e.byConnection.Close()
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

	byEndpoint := e.aggregations.EndpointAggregations
	for i, endpointAggregation := range byEndpoint {
		byStatus := endpointAggregation.StatsByStatusCode
		for _, s := range byStatus {
			s.Reset()
			httpStatsDataPool.Put(s)
		}

		// This is an idiom recognized and optimized by the Go compilar and results
		// in clearing the whole map at once
		// https://github.com/golang/go/issues/20138
		for k := range byStatus {
			delete(byStatus, k)
		}

		endpointAggregation.Reset()
		httpStatsPool.Put(endpointAggregation)
		byEndpoint[i] = nil
	}

	for i, mapPtr := range e.toRelease {
		httpStatusCodeMap.Put(mapPtr)
		e.toRelease[i] = nil
	}

	e.toRelease = e.toRelease[:0]
	e.aggregations.EndpointAggregations = e.aggregations.EndpointAggregations[:0]
}
