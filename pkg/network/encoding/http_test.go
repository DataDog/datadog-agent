// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	for i := 100; i <= 500; i += 100 {
		httpStats1.AddRequest(i, 10)
	}

	httpKey2 := httpKey1
	httpKey2.Path = "/testpath-2"
	var httpStats2 http.RequestStats
	for i := 100; i <= 500; i += 100 {
		httpStats2.AddRequest(i, 20)
	}

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: localhost,
					Dest:   localhost,
					SPort:  clientPort,
					DPort:  serverPort,
				},
			},
		},
		HTTP: map[http.Key]*http.RequestStats{
			httpKey1: &httpStats1,
			httpKey2: &httpStats2,
		},
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

	httpEncoder := newHTTPEncoder(in)
	aggregations := httpEncoder.GetHTTPAggregations(in.Conns[0])
	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations.EndpointAggregations)
}

func TestFormatHTTPStatsByPath(t *testing.T) {
	var httpReqStats http.RequestStats
	httpReqStats.AddRequest(100, 12.5)
	httpReqStats.AddRequest(100, 12.5)
	httpReqStats.AddRequest(405, 3.5)
	httpReqStats.AddRequest(405, 3.5)

	// Verify the latency data is correct prior to serialization
	latencies := httpReqStats.Stats(int(model.HTTPResponseStatus_Info+1) * 100).Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats.Stats(int(model.HTTPResponseStatus_ClientErr+1) * 100).Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	key := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		60000,
		80,
		"/testpath",
		http.MethodGet,
	)

	payload := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("10.2.2.2"),
					SPort:  60000,
					DPort:  80,
				},
			},
		},
		HTTP: map[http.Key]*http.RequestStats{
			key: &httpReqStats,
		},
	}
	httpEncoder := newHTTPEncoder(payload)
	httpAggregations := httpEncoder.GetHTTPAggregations(payload.Conns[0])

	require.NotNil(t, httpAggregations)
	endpointAggregations := httpAggregations.EndpointAggregations
	require.Len(t, endpointAggregations, 1)
	assert.Equal(t, "/testpath", endpointAggregations[0].Path)
	assert.Equal(t, model.HTTPMethod_Get, endpointAggregations[0].Method)

	// Deserialize the encoded latency information & confirm it is correct
	statsByResponseStatus := endpointAggregations[0].StatsByResponseStatus
	assert.Len(t, statsByResponseStatus, 5)

	serializedLatencies := statsByResponseStatus[model.HTTPResponseStatus_Info].Latencies
	sketch := unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 12.5)

	serializedLatencies = statsByResponseStatus[model.HTTPResponseStatus_ClientErr].Latencies
	sketch = unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 3.5)

	serializedLatencies = statsByResponseStatus[model.HTTPResponseStatus_Success].Latencies
	assert.Nil(t, serializedLatencies)
}

func unmarshalSketch(t *testing.T, bytes []byte) *ddsketch.DDSketch {
	var sketchPb sketchpb.DDSketch
	err := proto.Unmarshal(bytes, &sketchPb)
	assert.Nil(t, err)

	ret, err := ddsketch.FromProto(&sketchPb)
	assert.Nil(t, err)

	return ret
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
