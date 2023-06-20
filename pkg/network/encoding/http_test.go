// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"fmt"
	"runtime"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatHTTPStats(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testFormatHTTPStats(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testFormatHTTPStats(t, false)
	})
}

func testFormatHTTPStats(t *testing.T, aggregateByStatusCode bool) {
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
	httpStats1 := http.NewRequestStats(aggregateByStatusCode)
	for _, i := range statusCodes {
		httpStats1.AddRequest(i, 10, 1<<(i/100-1), nil)
	}

	httpKey2 := httpKey1
	httpKey2.Path = http.Path{
		Content:  "/testpath-2",
		FullPath: true,
	}
	httpStats2 := http.NewRequestStats(aggregateByStatusCode)
	for _, i := range statusCodes {
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
		HTTP: map[http.Key]*http.RequestStats{
			httpKey1: httpStats1,
			httpKey2: httpStats2,
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
		code := int32(httpStats1.NormalizeStatusCode(statusCode))
		out.EndpointAggregations[0].StatsByStatusCode[code] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 10, Latencies: nil}
		out.EndpointAggregations[1].StatsByStatusCode[code] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 20, Latencies: nil}
	}

	httpEncoder := newHTTPEncoder(in)
	aggregations, tags, _ := httpEncoder.GetHTTPAggregationsAndTags(in.Conns[0])
	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations.EndpointAggregations)

	// http.NumStatusClasses is the number of http class bucket of http.RequestStats
	// For this test we spread the bits (one per RequestStats) and httpStats1,2
	// and we test if all the bits has been aggregated together
	assert.Equal(t, uint64((1<<len(statusCodes))-1), tags)
}

func TestFormatHTTPStatsByPath(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testFormatHTTPStatsByPath(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testFormatHTTPStatsByPath(t, false)
	})
}

func testFormatHTTPStatsByPath(t *testing.T, aggregateByStatusCode bool) {
	httpReqStats := http.NewRequestStats(aggregateByStatusCode)

	httpReqStats.AddRequest(100, 12.5, 0, nil)
	httpReqStats.AddRequest(100, 12.5, tagGnuTLS, nil)
	httpReqStats.AddRequest(405, 3.5, tagOpenSSL, nil)
	httpReqStats.AddRequest(405, 3.5, 0, nil)

	// Verify the latency data is correct prior to serialization

	latencies := httpReqStats.Data[httpReqStats.NormalizeStatusCode(100)].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats.Data[httpReqStats.NormalizeStatusCode(405)].Latencies
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
		HTTP: map[http.Key]*http.RequestStats{
			key: httpReqStats,
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
	statsByResponseStatus := endpointAggregations[0].StatsByStatusCode
	assert.Len(t, statsByResponseStatus, 2)

	serializedLatencies := statsByResponseStatus[int32(httpReqStats.NormalizeStatusCode(100))].Latencies
	sketch := unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 12.5)

	serializedLatencies = statsByResponseStatus[int32(httpReqStats.NormalizeStatusCode(405))].Latencies
	sketch = unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 3.5)

	_, exists := statsByResponseStatus[200]
	assert.False(t, exists)
}

func TestIDCollisionRegression(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testIDCollisionRegression(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testIDCollisionRegression(t, false)
	})
}

func testIDCollisionRegression(t *testing.T, aggregateByStatusCode bool) {
	httpStats := http.NewRequestStats(aggregateByStatusCode)
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
	httpStats.AddRequest(104, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP: map[http.Key]*http.RequestStats{
			httpKey: httpStats,
		},
	}

	httpEncoder := newHTTPEncoder(in)

	// assert that the first connection matching the HTTP data will get
	// back a non-nil result
	aggregations, _, _ := httpEncoder.GetHTTPAggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(httpStats.NormalizeStatusCode(104))].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the HTTP stats
	// object
	aggregations, _, _ = httpEncoder.GetHTTPAggregationsAndTags(connections[1])
	assert.Nil(aggregations)
}

func TestLocalhostScenario(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testLocalhostScenario(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testLocalhostScenario(t, false)
	})
}

func testLocalhostScenario(t *testing.T, aggregateByStatusCode bool) {
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

	httpStats := http.NewRequestStats(aggregateByStatusCode)
	httpKey := http.NewKey(
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("127.0.0.1"),
		60000,
		80,
		"/",
		true,
		http.MethodGet,
	)
	httpStats.AddRequest(103, 1.0, 0, nil)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		HTTP: map[http.Key]*http.RequestStats{
			httpKey: httpStats,
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
			80,
			60000,
			"/",
			true,
			http.MethodGet,
		)

		in.HTTP[httpKeyWin] = httpStats
	}

	httpEncoder := newHTTPEncoder(in)

	// assert that both ends (client:server, server:client) of the connection
	// will have HTTP stats
	aggregations, _, _ := httpEncoder.GetHTTPAggregationsAndTags(connections[0])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(httpStats.NormalizeStatusCode(103))].Count)

	aggregations, _, _ = httpEncoder.GetHTTPAggregationsAndTags(connections[1])
	assert.NotNil(aggregations)
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(httpStats.NormalizeStatusCode(103))].Count)
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

func generateBenchMarkPayload(sourcePortsMax, destPortsMax uint16) network.Connections {
	localhost := util.AddressFromString("127.0.0.1")

	payload := network.Connections{
		BufferedData: network.BufferedData{
			Conns: make([]network.ConnectionStats, sourcePortsMax*destPortsMax),
		},
		HTTP: make(map[http.Key]*http.RequestStats),
	}

	httpStats := http.NewRequestStats(false)
	httpStats.AddRequest(100, 10, 0, nil)
	httpStats.AddRequest(200, 10, 0, nil)
	httpStats.AddRequest(300, 10, 0, nil)
	httpStats.AddRequest(400, 10, 0, nil)
	httpStats.AddRequest(500, 10, 0, nil)

	for sport := uint16(0); sport < sourcePortsMax; sport++ {
		for dport := uint16(0); dport < destPortsMax; dport++ {
			index := sport*sourcePortsMax + dport

			payload.Conns[index].Dest = localhost
			payload.Conns[index].Source = localhost
			payload.Conns[index].DPort = dport + 1
			payload.Conns[index].SPort = sport + 1
			if index%2 == 0 {
				payload.Conns[index].IPTranslation = &network.IPTranslation{
					ReplSrcIP:   localhost,
					ReplDstIP:   localhost,
					ReplSrcPort: dport + 1,
					ReplDstPort: sport + 1,
				}
			}

			payload.HTTP[http.NewKey(
				localhost,
				localhost,
				sport+1,
				dport+1,
				fmt.Sprintf("/api/%d-%d", sport+1, dport+1),
				true,
				http.MethodGet,
			)] = httpStats
		}
	}

	return payload
}

func commonBenchmarkHTTPEncoder(b *testing.B, numberOfPorts uint16) {
	payload := generateBenchMarkPayload(numberOfPorts, numberOfPorts)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		h := newHTTPEncoder(&payload)
		h.Close()
	}
}

func BenchmarkHTTPEncoder100Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 10)
}

func BenchmarkHTTPEncoder10000Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 100)
}
