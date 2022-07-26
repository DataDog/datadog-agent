// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
)

type httpEncoder struct {
	aggregations map[http.KeyTuple]*model.HTTPAggregations
	tags         map[http.KeyTuple]uint64

	// pre-allocated objects
	dataPool []model.HTTPStats_Data
	ptrPool  []*model.HTTPStats_Data
	poolIdx  int

	orphanEntries int
}

func newHTTPEncoder(payload *network.Connections) *httpEncoder {
	if len(payload.HTTP) == 0 {
		return nil
	}

	encoder := &httpEncoder{
		aggregations: make(map[http.KeyTuple]*model.HTTPAggregations, len(payload.Conns)),
		tags:         make(map[http.KeyTuple]uint64, len(payload.Conns)),

		// pre-allocate all data objects at once
		dataPool: make([]model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		ptrPool:  make([]*model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		poolIdx:  0,
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		encoder.aggregations[network.HTTPKeyTupleFromConn(conn)] = nil
	}

	encoder.buildAggregations(payload)
	return encoder
}

func (e *httpEncoder) GetHTTPAggregationsAndTags(c network.ConnectionStats) (*model.HTTPAggregations, uint64) {
	if e == nil {
		return nil, 0
	}

	keyTuple := network.HTTPKeyTupleFromConn(c)
	return e.aggregations[keyTuple], e.tags[keyTuple]
}

func (e *httpEncoder) buildAggregations(payload *network.Connections) {
	aggrSize := make(map[http.KeyTuple]int)
	for key := range payload.HTTP {
		aggrSize[key.KeyTuple]++
	}

	for key, stats := range payload.HTTP {
		aggregation, ok := e.aggregations[key.KeyTuple]
		if !ok {
			// if there is no matching connection don't even bother to serialize HTTP data
			e.orphanEntries++
			continue
		}

		if aggregation == nil {
			aggregation = &model.HTTPAggregations{
				EndpointAggregations: make([]*model.HTTPStats, 0, aggrSize[key.KeyTuple]),
			}
			e.aggregations[key.KeyTuple] = aggregation
		}

		ms := &model.HTTPStats{
			Path:                  key.Path.Content,
			FullPath:              key.Path.FullPath,
			Method:                model.HTTPMethod(key.Method),
			StatsByResponseStatus: e.getDataSlice(),
		}

		tags := e.tags[key.KeyTuple]
		for i, data := range ms.StatsByResponseStatus {
			class := (i + 1) * 100
			if !stats.HasStats(class) {
				continue
			}
			s := stats.Stats(class)
			data.Count = uint32(s.Count)

			if latencies := s.Latencies; latencies != nil {
				blob, _ := proto.Marshal(latencies.ToProto())
				data.Latencies = blob
			} else {
				data.FirstLatencySample = s.FirstLatencySample
			}

			tags |= s.Tags
		}

		e.tags[key.KeyTuple] = tags

		aggregation.EndpointAggregations = append(aggregation.EndpointAggregations, ms)
	}
}

func (e *httpEncoder) getDataSlice() []*model.HTTPStats_Data {
	ptrs := e.ptrPool[e.poolIdx : e.poolIdx+http.NumStatusClasses]
	for i := range ptrs {
		ptrs[i] = &e.dataPool[e.poolIdx+i]
	}
	e.poolIdx += http.NumStatusClasses
	return ptrs
}
