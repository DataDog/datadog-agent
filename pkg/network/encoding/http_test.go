// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/network/http/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatHTTPStats(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	httpKey1 := transaction.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		"/testpath-1",
		true,
		transaction.MethodGet,
	)
	var httpStats1 http.RequestStats
	for i := 100; i <= 500; i += 100 {
		httpStats1.AddRequest(i, 10, 1<<(i/100-1), nil)
	}

	httpKey2 := httpKey1
	httpKey2.Path = transaction.Path{
		Content:  "/testpath-2",
		FullPath: true,
	}
	var httpStats2 http.RequestStats
	for i := 100; i <= 500; i += 100 {
		httpStats2.AddRequest(i, 20, 1<<(i/100-1), nil)
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
		HTTP: map[transaction.Key]*http.RequestStats{
			httpKey1: &httpStats1,
			httpKey2: &httpStats2,
		},
	}
	out := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:     "/testpath-1",
				Method:   model.HTTPMethod_Get,
				FullPath: true,
				StatsByResponseStatus: []*model.HTTPStats_Data{
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
					{Count: 1, FirstLatencySample: 10, Latencies: nil},
				},
			},
			{
				Path:     "/testpath-2",
				Method:   model.HTTPMethod_Get,
				FullPath: true,
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
	aggregations, tags, _ := httpEncoder.GetHTTPAggregationsAndTags(in.Conns[0])
	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations.EndpointAggregations)

	// http.NumStatusClasses is the number of http class bucket of http.RequestStats
	// For this test we spread the bits (one per RequestStats) and httpStats1,2
	// and we test if all the bits has been aggregated together
	assert.Equal(t, uint64((1<<(http.NumStatusClasses))-1), tags)
}

func TestFormatHTTPStatsByPath(t *testing.T) {
	var httpReqStats http.RequestStats
	httpReqStats.AddRequest(100, 12.5, 0, nil)
	httpReqStats.AddRequest(100, 12.5, tagGnuTLS, nil)
	httpReqStats.AddRequest(405, 3.5, tagOpenSSL, nil)
	httpReqStats.AddRequest(405, 3.5, 0, nil)

	// Verify the latency data is correct prior to serialization
	latencies := httpReqStats.Stats(int(model.HTTPResponseStatus_Info+1) * 100).Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats.Stats(int(model.HTTPResponseStatus_ClientErr+1) * 100).Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	key := transaction.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		60000,
		80,
		"/testpath",
		true,
		transaction.MethodGet,
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
		HTTP: map[transaction.Key]*http.RequestStats{
			key: &httpReqStats,
		},
	}
	httpEncoder := newHTTPEncoder(payload)
	httpAggregations, tags, _ := httpEncoder.GetHTTPAggregationsAndTags(payload.Conns[0])

	require.NotNil(t, httpAggregations)
	endpointAggregations := httpAggregations.EndpointAggregations
	require.Len(t, endpointAggregations, 1)
	assert.Equal(t, "/testpath", endpointAggregations[0].Path)
	assert.Equal(t, model.HTTPMethod_Get, endpointAggregations[0].Method)

	assert.Equal(t, tagGnuTLS|tagOpenSSL, tags)

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

func TestIDCollisionRegression(t *testing.T) {
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    1,
		},
		{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    2,
		},
	}

	var httpStats http.RequestStats
	httpKey := transaction.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		"/",
		true,
		transaction.MethodGet,
	)
	httpStats.AddRequest(100, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP: map[transaction.Key]*http.RequestStats{
			httpKey: &httpStats,
		},
	}

	httpEncoder := newHTTPEncoder(in)

	// asssert that the first connection matching the the HTTP data will get
	// back a non-nil result
	aggregations, _, _ := httpEncoder.GetHTTPAggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByResponseStatus[0].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the HTTP stats
	// object
	aggregations, _, _ = httpEncoder.GetHTTPAggregationsAndTags(connections[1])
	assert.Nil(aggregations)
}

func TestLocalhostScenario(t *testing.T) {
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  80,
			Pid:    1,
		},
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  80,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  60000,
			Pid:    2,
		},
	}

	var httpStats http.RequestStats
	httpKey := transaction.NewKey(
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("127.0.0.1"),
		60000,
		80,
		"/",
		true,
		transaction.MethodGet,
	)
	httpStats.AddRequest(100, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP: map[transaction.Key]*http.RequestStats{
			httpKey: &httpStats,
		},
	}

	httpEncoder := newHTTPEncoder(in)

	// assert that both ends (client:server, server:client) of the connection
	// will have HTTP stats
	aggregations, _, _ := httpEncoder.GetHTTPAggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByResponseStatus[0].Count)

	aggregations, _, _ = httpEncoder.GetHTTPAggregationsAndTags(connections[1])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByResponseStatus[0].Count)
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
