// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"runtime"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRouteIdx(t *testing.T) {

	tests := []struct {
		desc                string
		via                 *network.Via
		routesIn, routesOut map[string]RouteIdx
		idx                 int32
	}{
		{
			desc: "nil via",
			via:  nil,
			idx:  -1,
		},
		{
			desc: "empty via",
			via:  &network.Via{},
			idx:  -1,
		},
		{
			desc:     "empty routes, non-nil via",
			via:      &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:      0,
			routesIn: map[string]RouteIdx{},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "foo"}},
			idx:  0,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
		},
		{
			desc: "non-empty routes, non-nil via with non-existing alias",
			via:  &network.Via{Subnet: network.Subnet{Alias: "bar"}},
			idx:  1,
			routesIn: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
			},
			routesOut: map[string]RouteIdx{
				"foo": {Idx: 0, Route: model.Route{Subnet: &model.Subnet{Alias: "foo"}}},
				"bar": {Idx: 1, Route: model.Route{Subnet: &model.Subnet{Alias: "bar"}}},
			},
		},
	}

	for _, te := range tests {
		t.Run(te.desc, func(t *testing.T) {
			idx := formatRouteIdx(te.via, te.routesIn)
			require.Equal(t, te.idx, idx)
			require.Len(t, te.routesIn, len(te.routesIn), "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			for k, v := range te.routesOut {
				otherv, ok := te.routesIn[k]
				require.True(t, ok, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
				require.Equal(t, v, otherv, "routes in and out are not equal, expected: %v, actual: %v", te.routesOut, te.routesIn)
			}
		})
	}
}

func TestFormatHTTPStats(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	httpKey1 := http.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		"/testpath-1",
		http.MethodGet,
	)
	var httpStats1 http.RequestStats
	for i := range httpStats1 {
		httpStats1[i].Count = 1
		httpStats1[i].FirstLatencySample = 10
		if i < len(httpStats1)/2 {
			httpStats1[i].Tags = (1 << i)
		}
	}

	httpKey2 := httpKey1
	httpKey2.Path = "/testpath-2"
	var httpStats2 http.RequestStats
	for i := range httpStats2 {
		httpStats2[i].Count = 1
		httpStats2[i].FirstLatencySample = 20
		if i >= len(httpStats1)/2 {
			httpStats2[i].Tags = (1 << i)
		}
	}

	in := map[http.Key]http.RequestStats{
		httpKey1: httpStats1,
		httpKey2: httpStats2,
	}
	out := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:   "/testpath-1",
				Method: model.HTTPMethod_Get,
				StatsByResponseStatus: []*model.HTTPStats_Data{
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
				},
			},
			{
				Path:   "/testpath-2",
				Method: model.HTTPMethod_Get,
				StatsByResponseStatus: []*model.HTTPStats_Data{
					{Count: 1, FirstLatencySample: 20, Latencies: nil},
					{Count: 1, FirstLatencySample: 20, Latencies: nil},
					{Count: 1, FirstLatencySample: 20, Latencies: nil},
					{Count: 1, FirstLatencySample: 20, Latencies: nil},
					{Count: 1, FirstLatencySample: 20, Latencies: nil},
				},
			},
		},
	}

	result, tags := FormatHTTPStats(in)

	aggregationKey := httpKey1
	aggregationKey.Path = ""
	aggregationKey.Method = http.MethodUnknown
	aggregations := result[aggregationKey].EndpointAggregations
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations)

	// http.NumStatusClasses is the number of http class bucket of http.RequestStats
	// For this test we spread the bits (one per RequestStats) and httpStats1,2
	// and we test if all the bits has been aggregated together
	assert.Equal(t, uint64((1<<(http.NumStatusClasses))-1), tags[aggregationKey])
}

func BenchmarkConnectionReset(b *testing.B) {
	c := new(model.Connection)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Reset()
	}
	runtime.KeepAlive(c)
}
