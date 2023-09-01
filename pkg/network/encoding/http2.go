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
	http2StatsDataPool = sync.Pool{
		New: func() any {
			return new(model.HTTPStats_Data)
		},
	}

	http2StatsPool = sync.Pool{
		New: func() any {
			return new(model.HTTPStats)
		},
	}

	http2StatusCodeMap = sync.Pool{
		New: func() any {
			m := make(map[int32]*model.HTTPStats_Data)
			return &m
		},
	}
)

type http2Encoder struct {
	byConnection *USMConnectionIndex[http.Key, *http.RequestStats]

	// cached object
	aggregations *model.HTTP2Aggregations

	// A list of pointers to maps of the protobuf representation. We get the pointers from sync.Pool, and by the end
	// of the operation, we put the objects back to the pool.
	toRelease []*map[int32]*model.HTTPStats_Data
}

func newHTTP2Encoder(http2Payloads map[http.Key]*http.RequestStats) *http2Encoder {
	if len(http2Payloads) == 0 {
		return nil
	}

	return &http2Encoder{
		byConnection: GroupByConnection("http2", http2Payloads, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		}),
		aggregations: new(model.HTTP2Aggregations),
	}
}

func (e *http2Encoder) GetHTTP2AggregationsAndTags(c network.ConnectionStats) ([]byte, uint64, map[string]struct{}) {
	if e == nil {
		return nil, 0, nil
	}

	connectionData := e.byConnection.Find(c)
	if connectionData == nil || len(connectionData.Data) == 0 || connectionData.IsPIDCollision(c) {
		return nil, 0, nil
	}

	return e.encodeData(connectionData)
}

func (e *http2Encoder) encodeData(connectionData *USMConnectionData[http.Key, *http.RequestStats]) ([]byte, uint64, map[string]struct{}) {
	e.reset()

	var staticTags uint64
	dynamicTags := make(map[string]struct{})

	for _, kvPair := range connectionData.Data {
		key := kvPair.Key
		stats := kvPair.Value

		ms := http2StatsPool.Get().(*model.HTTPStats)
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

func (e *http2Encoder) Close() {
	if e == nil {
		return
	}

	e.reset()
	e.byConnection.Close()
}

func (e *http2Encoder) getDataMap(stats map[uint16]*http.RequestStat) map[int32]*model.HTTPStats_Data {
	resPtr := http2StatusCodeMap.Get().(*map[int32]*model.HTTPStats_Data)
	e.toRelease = append(e.toRelease, resPtr)

	res := *resPtr
	for key := range stats {
		res[int32(key)] = http2StatsDataPool.Get().(*model.HTTPStats_Data)
	}
	return res
}

func (e *http2Encoder) reset() {
	if e == nil {
		return
	}

	byEndpoint := e.aggregations.EndpointAggregations
	for i, endpointAggregation := range byEndpoint {
		byStatus := endpointAggregation.StatsByStatusCode
		for _, s := range byStatus {
			s.Reset()
			http2StatsDataPool.Put(s)
		}

		// This is an idiom recognized and optimized by the Go compiler and results
		// in clearing the whole map at once
		// https://github.com/golang/go/issues/20138
		for k := range byStatus {
			delete(byStatus, k)
		}

		endpointAggregation.Reset()
		http2StatsPool.Put(endpointAggregation)
		byEndpoint[i] = nil
	}

	for i, mapPtr := range e.toRelease {
		http2StatusCodeMap.Put(mapPtr)
		e.toRelease[i] = nil
	}

	e.toRelease = e.toRelease[:0]
	e.aggregations.EndpointAggregations = e.aggregations.EndpointAggregations[:0]
}
