// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"runtime"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatHTTP2Stats(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testFormatHTTP2Stats(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testFormatHTTP2Stats(t, false)
	})
}

func testFormatHTTP2Stats(t *testing.T, aggregateByStatusCode bool) {
	var (
		clientPort  = uint16(52800)
		serverPort  = uint16(8080)
		localhost   = util.AddressFromString("127.0.0.1")
		statusCodes = []uint16{101, 202, 307, 404, 503}
	)

	httpKey1 := http.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		"/testpath-1",
		true,
		http.MethodGet,
	)
	http2Stats1 := http.NewRequestStats(aggregateByStatusCode)
	for _, i := range statusCodes {
		http2Stats1.AddRequest(i, 10, 1<<(i/100-1), nil)
	}

	httpKey2 := httpKey1
	httpKey2.Path = http.Path{
		Content:  "/testpath-2",
		FullPath: true,
	}
	http2Stats2 := http.NewRequestStats(aggregateByStatusCode)
	for _, i := range statusCodes {
		http2Stats2.AddRequest(i, 20, 1<<(i/100-1), nil)
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
		HTTP2: map[http.Key]*http.RequestStats{
			httpKey1: http2Stats1,
			httpKey2: http2Stats2,
		},
	}
	out := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:              "/testpath-1",
				Method:            model.HTTPMethod_Get,
				FullPath:          true,
				StatsByStatusCode: make(map[int32]*model.HTTPStats_Data),
			},
			{
				Path:              "/testpath-2",
				Method:            model.HTTPMethod_Get,
				FullPath:          true,
				StatsByStatusCode: make(map[int32]*model.HTTPStats_Data),
			},
		},
	}

	for _, statusCode := range statusCodes {
		code := int32(http2Stats1.NormalizeStatusCode(statusCode))
		out.EndpointAggregations[0].StatsByStatusCode[code] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 10, Latencies: nil}
		out.EndpointAggregations[1].StatsByStatusCode[code] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 20, Latencies: nil}
	}

	http2Encoder := newHTTP2Encoder(in)
	aggregations, tags, _ := http2Encoder.GetHTTP2AggregationsAndTags(in.Conns[0])
	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations.EndpointAggregations)

	assert.Equal(t, uint64((1<<len(statusCodes))-1), tags)
}

func TestFormatHTTP2StatsByPath(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testFormatHTTP2StatsByPath(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testFormatHTTP2StatsByPath(t, false)
	})
}

func testFormatHTTP2StatsByPath(t *testing.T, aggregateByStatusCode bool) {
	http2ReqStats := http.NewRequestStats(aggregateByStatusCode)

	http2ReqStats.AddRequest(100, 12.5, 0, nil)
	http2ReqStats.AddRequest(100, 12.5, tagGnuTLS, nil)
	http2ReqStats.AddRequest(405, 3.5, tagOpenSSL, nil)
	http2ReqStats.AddRequest(405, 3.5, 0, nil)

	// Verify the latency data is correct prior to serialization

	latencies := http2ReqStats.Data[http2ReqStats.NormalizeStatusCode(100)].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = http2ReqStats.Data[http2ReqStats.NormalizeStatusCode(405)].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	key := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		60000,
		80,
		"/testpath",
		true,
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
		HTTP2: map[http.Key]*http.RequestStats{
			key: http2ReqStats,
		},
	}
	http2Encoder := newHTTP2Encoder(payload)
	httpAggregations, tags, _ := http2Encoder.GetHTTP2AggregationsAndTags(payload.Conns[0])

	require.NotNil(t, httpAggregations)
	endpointAggregations := httpAggregations.EndpointAggregations
	require.Len(t, endpointAggregations, 1)
	assert.Equal(t, "/testpath", endpointAggregations[0].Path)
	assert.Equal(t, model.HTTPMethod_Get, endpointAggregations[0].Method)

	assert.Equal(t, tagGnuTLS|tagOpenSSL, tags)

	// Deserialize the encoded latency information & confirm it is correct
	statsByResponseStatus := endpointAggregations[0].StatsByStatusCode
	assert.Len(t, statsByResponseStatus, 2)

	serializedLatencies := statsByResponseStatus[int32(http2ReqStats.NormalizeStatusCode(100))].Latencies
	sketch := unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 12.5)

	serializedLatencies = statsByResponseStatus[int32(http2ReqStats.NormalizeStatusCode(405))].Latencies
	sketch = unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 3.5)

	_, exists := statsByResponseStatus[200]
	assert.False(t, exists)
}

func TestIDCollisionRegressionHTTP2(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testIDCollisionRegressionHTTP2(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testIDCollisionRegressionHTTP2(t, false)
	})
}

func testIDCollisionRegressionHTTP2(t *testing.T, aggregateByStatusCode bool) {
	http2Stats := http.NewRequestStats(aggregateByStatusCode)
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

	httpKey := http.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		"/",
		true,
		http.MethodGet,
	)
	http2Stats.AddRequest(104, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP2: map[http.Key]*http.RequestStats{
			httpKey: http2Stats,
		},
	}

	http2Encoder := newHTTP2Encoder(in)

	// assert that the first connection matching the HTTP data will get
	// back a non-nil result
	aggregations, _, _ := http2Encoder.GetHTTP2AggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(http2Stats.NormalizeStatusCode(104))].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the HTTP stats
	// object
	aggregations, _, _ = http2Encoder.GetHTTP2AggregationsAndTags(connections[1])
	assert.Nil(aggregations)
}

func TestHTTP2LocalhostScenario(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testHTTP2LocalhostScenario(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testHTTP2LocalhostScenario(t, false)
	})
}

func testHTTP2LocalhostScenario(t *testing.T, aggregateByStatusCode bool) {
	assert := assert.New(t)
	cliport := uint16(6000)
	serverport := uint16(80)
	connections := []network.ConnectionStats{
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  cliport,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  serverport,
			Pid:    1,
		},
		{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  serverport,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  cliport,
			Pid:    2,
		},
	}

	http2Stats := http.NewRequestStats(aggregateByStatusCode)
	httpKey := http.NewKey(
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("127.0.0.1"),
		cliport,
		serverport,
		"/",
		true,
		http.MethodGet,
	)

	http2Stats.AddRequest(103, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP2: map[http.Key]*http.RequestStats{
			httpKey: http2Stats,
		},
	}
	if runtime.GOOS == "windows" {
		/*
		 * on Windows, there are separate http transactions for
		 * each side of the connection.  And they're kept separate,
		 * and keyed separately.  Address this condition until the
		 * platforms are resynced
		 */
		httpKeyWin := http.NewKey(
			util.AddressFromString("127.0.0.1"),
			util.AddressFromString("127.0.0.1"),
			serverport,
			cliport,
			"/",
			true,
			http.MethodGet,
		)

		in.HTTP2[httpKeyWin] = http2Stats
	}
	http2Encoder := newHTTP2Encoder(in)

	// assert that both ends (client:server, server:client) of the connection
	// will have HTTP2 stats
	aggregations, _, _ := http2Encoder.GetHTTP2AggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(http2Stats.NormalizeStatusCode(103))].Count)

	aggregations, _, _ = http2Encoder.GetHTTP2AggregationsAndTags(connections[1])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(http2Stats.NormalizeStatusCode(103))].Count)
}
