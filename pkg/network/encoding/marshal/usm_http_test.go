// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package marshal

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
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/types"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestFormatHTTPStats(t *testing.T) {
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
		[]byte("/testpath-1"),
		true,
		http.MethodGet,
	)
	httpStats1 := http.NewRequestStats()
	for _, i := range statusCodes {
		httpStats1.AddRequest(i, 10, 1<<(i/100-1), nil)
	}

	httpKey2 := httpKey1
	httpKey2.Path = http.Path{
		Content:  http.Interner.GetString("/testpath-2"),
		FullPath: true,
	}
	httpStats2 := http.NewRequestStats()
	for _, i := range statusCodes {
		httpStats2.AddRequest(i, 20, 1<<(i/100-1), nil)
	}

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: localhost,
					Dest:   localhost,
					SPort:  clientPort,
					DPort:  serverPort,
				}},
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
		out.EndpointAggregations[0].StatsByStatusCode[int32(statusCode)] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 10, Latencies: nil}
		out.EndpointAggregations[1].StatsByStatusCode[int32(statusCode)] = &model.HTTPStats_Data{Count: 1, FirstLatencySample: 20, Latencies: nil}
	}

	httpEncoder := newHTTPEncoder(in.HTTP)
	aggregations, tags, _ := getHTTPAggregations(t, httpEncoder, in.Conns[0])

	require.NotNil(t, aggregations)
	assert.ElementsMatch(t, out.EndpointAggregations, aggregations.EndpointAggregations)

	// http.NumStatusClasses is the number of http class bucket of http.RequestStats
	// For this test we spread the bits (one per RequestStats) and httpStats1,2
	// and we test if all the bits has been aggregated together
	assert.Equal(t, uint64((1<<len(statusCodes))-1), tags)
}

func TestFormatHTTPStatsByPath(t *testing.T) {
	httpReqStats := http.NewRequestStats()

	httpReqStats.AddRequest(100, 12.5, 0, nil)
	httpReqStats.AddRequest(100, 12.5, tagGnuTLS, nil)
	httpReqStats.AddRequest(405, 3.5, tagOpenSSL, nil)
	httpReqStats.AddRequest(405, 3.5, 0, nil)

	// Verify the latency data is correct prior to serialization

	latencies := httpReqStats.Data[100].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats.Data[405].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	key := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		60000,
		80,
		[]byte("/testpath"),
		true,
		http.MethodGet,
	)

	payload := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("10.2.2.2"),
					SPort:  60000,
					DPort:  80,
				}},
			},
		},
		HTTP: map[http.Key]*http.RequestStats{
			key: httpReqStats,
		},
	}
	httpEncoder := newHTTPEncoder(payload.HTTP)
	httpAggregations, tags, _ := getHTTPAggregations(t, httpEncoder, payload.Conns[0])

	require.NotNil(t, httpAggregations)
	endpointAggregations := httpAggregations.EndpointAggregations
	require.Len(t, endpointAggregations, 1)
	assert.Equal(t, "/testpath", endpointAggregations[0].Path)
	assert.Equal(t, model.HTTPMethod_Get, endpointAggregations[0].Method)

	assert.Equal(t, tagGnuTLS|tagOpenSSL, tags)

	// Deserialize the encoded latency information & confirm it is correct
	statsByResponseStatus := endpointAggregations[0].StatsByStatusCode
	assert.Len(t, statsByResponseStatus, 2)

	serializedLatencies := statsByResponseStatus[int32(100)].Latencies
	sketch := unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 12.5)

	serializedLatencies = statsByResponseStatus[int32(405)].Latencies
	sketch = unmarshalSketch(t, serializedLatencies)
	assert.Equal(t, 2.0, sketch.GetCount())
	verifyQuantile(t, sketch, 0.5, 3.5)

	_, exists := statsByResponseStatus[200]
	assert.False(t, exists)
}

func TestIDCollisionRegression(t *testing.T) {
	httpStats := http.NewRequestStats()
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("1.1.1.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("2.2.2.2"),
			DPort:  80,
			Pid:    2,
		}},
	}

	httpKey := http.NewKey(
		util.AddressFromString("1.1.1.1"),
		util.AddressFromString("2.2.2.2"),
		60000,
		80,
		[]byte("/"),
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

	httpEncoder := newHTTPEncoder(in.HTTP)

	// assert that the first connection matching the HTTP data will get
	// back a non-nil result
	aggregations, _, _ := getHTTPAggregations(t, httpEncoder, in.Conns[0])
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(104)].Count)

	// assert that the other connections sharing the same (source,destination)
	// addresses but different PIDs *won't* be associated with the HTTP stats
	// object
	streamer := NewProtoTestStreamer[*model.Connection]()
	httpEncoder.GetHTTPAggregationsAndTags(connections[1], model.NewConnectionBuilder(streamer))

	var conn model.Connection
	streamer.Unwrap(t, &conn)
	assert.Empty(conn.HttpAggregations)
}

func TestLocalhostScenario(t *testing.T) {
	assert := assert.New(t)
	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  60000,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  80,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: util.AddressFromString("127.0.0.1"),
			SPort:  80,
			Dest:   util.AddressFromString("127.0.0.1"),
			DPort:  60000,
			Pid:    2,
		}},
	}

	httpStats := http.NewRequestStats()
	httpKey := http.NewKey(
		util.AddressFromString("127.0.0.1"),
		util.AddressFromString("127.0.0.1"),
		60000,
		80,
		[]byte("/"),
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
			[]byte("/"),
			true,
			http.MethodGet,
		)

		in.HTTP[httpKeyWin] = httpStats
	}

	httpEncoder := newHTTPEncoder(in.HTTP)

	// assert that both ends (client:server, server:client) of the connection
	// will have HTTP stats
	aggregations, _, _ := getHTTPAggregations(t, httpEncoder, in.Conns[0])
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(103)].Count)

	aggregations, _, _ = getHTTPAggregations(t, httpEncoder, in.Conns[1])
	assert.Equal("/", aggregations.EndpointAggregations[0].Path)
	assert.Equal(uint32(1), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(103)].Count)
}

// TestKubernetesNATScenario tests how USMConnectionIndex handles Kubernetes-style NAT connections
// with both pre-NAT and post-NAT connections as seen in Kubernetes environments
func TestKubernetesNATScenario(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping test on Windows")
	}
	assert := assert.New(t)

	// Create IP addresses and ports exactly as seen in the logs
	clientIP := util.AddressFromString("172.29.161.37")   // Client IP from logs
	serviceIP := util.AddressFromString("10.100.103.122") // Service IP (ClusterIP) from logs
	serverIP := util.AddressFromString("172.29.191.94")   // Server IP from logs
	clientPort := uint16(53792)                           // Client port from logs
	servicePort := uint16(7778)                           // Service port from logs
	serverPort := uint16(7777)                            // Server port from logs

	// Create both connections exactly as seen in the logs
	connections := []network.ConnectionStats{
		{ // Pre-NAT connection: client -> service
			ConnectionTuple: network.ConnectionTuple{
				Source:    clientIP,
				SPort:     clientPort,
				Dest:      serviceIP,
				DPort:     servicePort,
				Pid:       1784258, // PID from logs
				Type:      network.TCP,
				Direction: network.OUTGOING,
			},
			ProtocolStack: protocols.Stack{Application: protocols.HTTP},
			IPTranslation: &network.IPTranslation{
				// Values from logs - NOTE: this is flipped compared to what one might expect
				// The ReplSrc fields contain the server endpoint, and ReplDst contains the client
				ReplSrcIP:   serverIP,   // Server IP in ReplSrcIP (from logs)
				ReplSrcPort: serverPort, // Server port in ReplSrcPort (from logs)
				ReplDstIP:   clientIP,   // Client IP in ReplDstIP (from logs)
				ReplDstPort: clientPort, // Client port in ReplDstPort (from logs)
			},
		},
		{ // Post-NAT connection: server -> client (from server's perspective)
			ConnectionTuple: network.ConnectionTuple{
				Source:    serverIP,   // Server is the source in this connection
				SPort:     serverPort, // Server port
				Dest:      clientIP,   // Client is the destination
				DPort:     clientPort, // Client port
				Pid:       334392,     // PID from logs
				Type:      network.TCP,
				Direction: network.INCOMING,
			},
			ProtocolStack: protocols.Stack{Application: protocols.HTTP},
			// No IPTranslation for this connection as seen in logs
		},
	}

	// Create HTTP stats with a sample request - 4 requests as seen in logs
	httpStats := http.NewRequestStats()
	httpStats.AddRequest(200, 15.0, 0, nil)
	httpStats.AddRequest(200, 15.0, 0, nil)
	httpStats.AddRequest(200, 15.0, 0, nil)
	httpStats.AddRequest(200, 15.0, 0, nil)

	// Create pre-NAT HTTP key (client → service) with path from logs
	preNATKey := http.NewKey(
		clientIP,
		serviceIP,
		clientPort,
		servicePort,
		[]byte("/delay/5"),
		true,
		http.MethodGet,
	)

	// Create post-NAT HTTP key (client → server) with path from logs
	// Note: According to the logs, even post-NAT HTTP key has client as source and server as destination
	postNATKey := http.NewKey(
		clientIP,   // Client is still the source in the HTTP key (172.29.161.37)
		serverIP,   // Server is the destination (172.29.191.94)
		clientPort, // Client port (53792)
		serverPort, // Server port (7777)
		[]byte("/delay/5"),
		true,
		http.MethodGet,
	)

	// Test scenario 1: Both pre-NAT and post-NAT keys in the HTTP data
	t.Run("Both pre-NAT and post-NAT keys in HTTP data", func(t *testing.T) {
		httpData := map[http.Key]*http.RequestStats{
			preNATKey:  httpStats,
			postNATKey: httpStats,
		}

		// Create connection payload
		payload := &network.Connections{
			BufferedData: network.BufferedData{
				Conns: connections,
			},
			HTTP: httpData,
		}

		// Create HTTP encoder
		httpEncoder := newHTTPEncoder(payload.HTTP)

		// Test pre-NAT connection (client → service)
		aggregations, _, _ := getHTTPAggregations(t, httpEncoder, payload.Conns[0])
		assert.NotNil(aggregations)
		assert.Equal("/delay/5", aggregations.EndpointAggregations[0].Path)
		assert.Equal(model.HTTPMethod_Get, aggregations.EndpointAggregations[0].Method)
		assert.Equal(uint32(4), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(200)].Count)

		// Test post-NAT connection (server → client)
		aggregations, _, _ = getHTTPAggregations(t, httpEncoder, payload.Conns[1])
		assert.NotNil(aggregations)
		assert.Equal("/delay/5", aggregations.EndpointAggregations[0].Path)
		assert.Equal(model.HTTPMethod_Get, aggregations.EndpointAggregations[0].Method)
		assert.Equal(uint32(4), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(200)].Count)

		// Create USMConnectionIndex for direct testing
		httpIndex := GroupByConnection("http", httpData, func(key http.Key) types.ConnectionKey {
			return key.ConnectionKey
		})

		// Test direct lookup in the index
		preNATResult := httpIndex.Find(payload.Conns[0])
		assert.NotNil(preNATResult, "Pre-NAT connection (client → service) should be found directly")

		postNATResult := httpIndex.Find(payload.Conns[1])
		assert.NotNil(postNATResult, "Post-NAT connection (server → client) should be found directly")
	})

	// Test scenario 2: Only pre-NAT key in the HTTP data
	t.Run("Only pre-NAT key in HTTP data", func(t *testing.T) {
		httpData := map[http.Key]*http.RequestStats{
			preNATKey: httpStats,
		}

		// Create connection payload
		payload := &network.Connections{
			BufferedData: network.BufferedData{
				Conns: connections,
			},
			HTTP: httpData,
		}

		// Create HTTP encoder
		httpEncoder := newHTTPEncoder(payload.HTTP)

		// Test pre-NAT connection (client → service) - should have HTTP data
		aggregations, _, _ := getHTTPAggregations(t, httpEncoder, payload.Conns[0])
		assert.NotNil(aggregations)
		assert.Equal("/delay/5", aggregations.EndpointAggregations[0].Path)
		assert.Equal(uint32(4), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(200)].Count)

		// Test post-NAT connection (server → client) - should NOT have HTTP data since direct match not available
		streamer := NewProtoTestStreamer[*model.Connection]()
		httpEncoder.GetHTTPAggregationsAndTags(payload.Conns[1], model.NewConnectionBuilder(streamer))

		var conn model.Connection
		streamer.Unwrap(t, &conn)
		assert.Empty(conn.HttpAggregations, "Post-NAT connection (server → client) should not have HTTP data with only pre-NAT in the index")

		// Test manual lookup with the generated index
		t.Log("In a real K8s environment, we would need NAT enrichment to find data for the post-NAT connection")
	})

	// Test scenario 3: Only post-NAT key in the HTTP data
	t.Run("Only post-NAT key in HTTP data", func(t *testing.T) {
		httpData := map[http.Key]*http.RequestStats{
			postNATKey: httpStats,
		}

		// Create connection payload
		payload := &network.Connections{
			BufferedData: network.BufferedData{
				Conns: connections,
			},
			HTTP: httpData,
		}

		// Create HTTP encoder
		httpEncoder := newHTTPEncoder(payload.HTTP)

		// Test post-NAT connection (server → client) - should have HTTP data
		aggregations, _, _ := getHTTPAggregations(t, httpEncoder, payload.Conns[1])
		assert.NotNil(aggregations)
		assert.Equal("/delay/5", aggregations.EndpointAggregations[0].Path)
		assert.Equal(uint32(4), aggregations.EndpointAggregations[0].StatsByStatusCode[int32(200)].Count)

		// Test pre-NAT connection (client → service) - should NOT have HTTP data with only post-NAT in the index
		streamer := NewProtoTestStreamer[*model.Connection]()
		httpEncoder.GetHTTPAggregationsAndTags(payload.Conns[0], model.NewConnectionBuilder(streamer))

		var conn model.Connection
		streamer.Unwrap(t, &conn)
		assert.Empty(conn.HttpAggregations, "Pre-NAT connection (client → service) should not have HTTP data with only post-NAT in the index")

		// Test manual lookup with the generated index
		t.Log("In a real K8s environment, we would need NAT enrichment to find data for the pre-NAT connection")
	})
}

func getHTTPAggregations(t *testing.T, encoder *httpEncoder, c network.ConnectionStats) (*model.HTTPAggregations, uint64, map[string]struct{}) {
	streamer := NewProtoTestStreamer[*model.Connection]()
	staticTags, dynamicTags := encoder.GetHTTPAggregationsAndTags(c, model.NewConnectionBuilder(streamer))

	var conn model.Connection
	streamer.Unwrap(t, &conn)

	var aggregations model.HTTPAggregations
	err := proto.Unmarshal(conn.HttpAggregations, &aggregations)
	require.NoError(t, err)

	return &aggregations, staticTags, dynamicTags
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

	httpStats := http.NewRequestStats()
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
				[]byte(fmt.Sprintf("/api/%d-%d", sport+1, dport+1)),
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
	var h *httpEncoder
	for i := 0; i < b.N; i++ {
		h = newHTTPEncoder(payload.HTTP)
	}
	runtime.KeepAlive(h)
}

func BenchmarkHTTPEncoder100Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 10)
}

func BenchmarkHTTPEncoder10000Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 100)
}
