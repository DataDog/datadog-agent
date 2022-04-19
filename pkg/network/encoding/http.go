// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/gogo/protobuf/proto"
)

type httpEncoder struct {
	aggregations map[http.Key]*model.HTTPAggregations

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
		aggregations: make(map[http.Key]*model.HTTPAggregations, len(payload.Conns)),

		// pre-allocate all data objects at once
		dataPool: make([]model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		ptrPool:  make([]*model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		poolIdx:  0,
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		encoder.aggregations[httpKeyFromConn(conn)] = nil
	}

	encoder.buildAggregations(payload)
	return encoder
}

func (e *httpEncoder) GetHTTPAggregations(c network.ConnectionStats) *model.HTTPAggregations {
	if e == nil {
		return nil
	}

	return e.aggregations[httpKeyFromConn(c)]
}

func (e *httpEncoder) buildAggregations(payload *network.Connections) {
	for key, stats := range payload.HTTP {
		path := key.Path
		method := key.Method
		key.Path = ""
		key.Method = http.MethodUnknown

		aggregation, ok := e.aggregations[key]
		if !ok {
			// if there is no matching connection don't even bother to serialize HTTP data
			e.orphanEntries++
			continue
		}

		if aggregation == nil {
			aggregation = &model.HTTPAggregations{
				EndpointAggregations: make([]*model.HTTPStats, 0, 10),
			}
			e.aggregations[key] = aggregation
		}

		ms := &model.HTTPStats{
			Path:                  path,
			Method:                model.HTTPMethod(method),
			StatsByResponseStatus: e.getDataSlice(),
		}

		for i, data := range ms.StatsByResponseStatus {
			data.Count = uint32(stats[i].Count)

			if latencies := stats[i].Latencies; latencies != nil {
				blob, _ := proto.Marshal(latencies.ToProto())
				data.Latencies = blob
			} else {
				data.FirstLatencySample = stats[i].FirstLatencySample
			}
		}

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

// build the key for the http map based on whether the local or remote side is http.
func httpKeyFromConn(c network.ConnectionStats) http.Key {
	// Retrieve translated addresses
	laddr, lport := network.GetNATLocalAddress(c)
	raddr, rport := network.GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), so we flip
	// the lookup key if necessary using the port range heuristic
	if network.IsEphemeralPort(int(lport)) {
		return http.NewKey(laddr, raddr, lport, rport, "", http.MethodUnknown)
	}

	return http.NewKey(raddr, laddr, rport, lport, "", http.MethodUnknown)
}
