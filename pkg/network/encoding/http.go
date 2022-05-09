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
	aggregations map[http.KeyTuple]*model.HTTPAggregations

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

		// pre-allocate all data objects at once
		dataPool: make([]model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		ptrPool:  make([]*model.HTTPStats_Data, len(payload.HTTP)*http.NumStatusClasses),
		poolIdx:  0,
	}

	// pre-populate aggregation map with keys for all existent connections
	// this allows us to skip encoding orphan HTTP objects that can't be matched to a connection
	for _, conn := range payload.Conns {
		encoder.aggregations[httpKeyTupleFromConn(conn)] = nil
	}

	encoder.buildAggregations(payload)
	return encoder
}

func (e *httpEncoder) GetHTTPAggregations(c network.ConnectionStats) *model.HTTPAggregations {
	if e == nil {
		return nil
	}

	return e.aggregations[httpKeyTupleFromConn(c)]
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
			Path:                  key.Path,
			Method:                model.HTTPMethod(key.Method),
			StatsByResponseStatus: e.getDataSlice(),
		}

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

// Build the key for the http map based on whether the local or remote side is http.
func httpKeyTupleFromConn(c network.ConnectionStats) http.KeyTuple {
	// Retrieve translated addresses
	laddr, lport := network.GetNATLocalAddress(c)
	raddr, rport := network.GetNATRemoteAddress(c)

	// HTTP data is always indexed as (client, server), so we account for that when generating the
	// the lookup key using the port range heuristic.
	// In the rare cases where both ports are within the same range we ensure that sport < dport
	// to mimic the normalization heuristic done in the eBPF side (see `port_range.h`)
	if (network.IsEphemeralPort(int(lport)) && !network.IsEphemeralPort(int(rport))) ||
		(network.IsEphemeralPort(int(lport)) == network.IsEphemeralPort(int(rport)) && lport < rport) {
		return http.NewKeyTuple(laddr, raddr, lport, rport)
	}

	return http.NewKeyTuple(raddr, laddr, rport, lport)
}
