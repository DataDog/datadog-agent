package encoding

import (
	"encoding/json"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSerialization(t *testing.T) {
	var httpReqStats http.RequestStats
	in := &network.Connections{
		Conns: []network.ConnectionStats{
			{
				Source:               util.AddressFromString("10.1.1.1"),
				Dest:                 util.AddressFromString("10.2.2.2"),
				MonotonicSentBytes:   1,
				LastSentBytes:        2,
				MonotonicRecvBytes:   100,
				LastRecvBytes:        101,
				LastUpdateEpoch:      50,
				LastTCPEstablished:   1,
				LastTCPClosed:        1,
				MonotonicRetransmits: 201,
				LastRetransmits:      201,
				Pid:                  6000,
				NetNS:                7,
				SPort:                1000,
				DPort:                9000,
				IPTranslation: &network.IPTranslation{
					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
					ReplDstIP:   util.AddressFromString("20.1.1.1"),
					ReplSrcPort: 40,
					ReplDstPort: 80,
				},

				Type:      network.UDP,
				Family:    network.AFINET6,
				Direction: network.LOCAL,

				DNSCountByRcode: map[uint32]uint32{0: 1},
				DNSStatsByDomain: map[string]network.DNSStats{
					"foo.com": {
						DNSTimeouts:          0,
						DNSSuccessLatencySum: 0,
						DNSFailureLatencySum: 0,
						DNSCountByRcode:      map[uint32]uint32{0: 1},
					},
				},
				Via: &network.Via{
					Subnet: network.Subnet{
						Alias: "subnet-foo",
					},
				},
			},
		},
		DNS: map[util.Address][]string{
			util.AddressFromString("172.217.12.145"): {"golang.org"},
		},
		HTTP: map[http.Key]http.RequestStats{
			http.NewKey(
				util.AddressFromString("20.1.1.1"),
				util.AddressFromString("20.1.1.1"),
				40,
				80,
				"/testpath",
				http.MethodGet,
			): httpReqStats,
		},
	}

	httpOut := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:   "/testpath",
				Method: model.HTTPMethod_Get,
				StatsByResponseStatus: []*model.HTTPStats_Data{
					{
						Count:     0,
						Latencies: nil,
					},
					{
						Count:     0,
						Latencies: nil,
					},
					{
						Count:     0,
						Latencies: nil,
					},
					{
						Count:     0,
						Latencies: nil,
					},
					{
						Count:     0,
						Latencies: nil,
					},
				},
			},
		},
	}

	httpOutBlob, err := proto.Marshal(httpOut)
	require.NoError(t, err)

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:              &model.Addr{Ip: "10.1.1.1", Port: int32(1000)},
				Raddr:              &model.Addr{Ip: "10.2.2.2", Port: int32(9000)},
				LastBytesSent:      2,
				LastBytesReceived:  101,
				LastRetransmits:    201,
				LastTcpEstablished: 1,
				LastTcpClosed:      1,
				Pid:                int32(6000),
				NetNS:              7,
				IpTranslation: &model.IPTranslation{
					ReplSrcIP:   "20.1.1.1",
					ReplDstIP:   "20.1.1.1",
					ReplSrcPort: int32(40),
					ReplDstPort: int32(80),
				},

				Type:      model.ConnectionType_udp,
				Family:    model.ConnectionFamily_v6,
				Direction: model.ConnectionDirection_local,

				DnsCountByRcode: map[uint32]uint32{0: 1},
				DnsStatsByDomain: map[int32]*model.DNSStats{
					0: {
						DnsTimeouts:          0,
						DnsSuccessLatencySum: 0,
						DnsFailureLatencySum: 0,
						DnsCountByRcode:      map[uint32]uint32{0: 1},
					},
				},
				RouteIdx:         0,
				HttpAggregations: httpOutBlob,
			},
		},
		Dns: map[string]*model.DNSEntry{
			"172.217.12.145": {Names: []string{"golang.org"}},
		},
		Domains: []string{"foo.com"},
		Routes: []*model.Route{
			{
				Subnet: &model.Subnet{
					Alias: "subnet-foo",
				},
			},
		},
		CompilationTelemetryByAsset: map[string]*model.RuntimeCompilationTelemetry{},
	}

	t.Run("requesting application/json serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("requesting empty serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("")
		// in case we request empty serialization type, default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("requesting unsupported serialization format", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/whatever")

		// In case we request an unsupported serialization type, we default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)
		assert.Equal(out, result)
	})

	t.Run("render default values with application/json", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		// Empty connection batch
		blob, err := marshaler.Marshal(&network.Connections{Conns: []network.ConnectionStats{{}}})
		require.NoError(t, err)

		res := struct {
			Conns []map[string]interface{} `json:"conns"`
		}{}
		require.NoError(t, json.Unmarshal(blob, &res))

		require.Len(t, res.Conns, 1)
		// Check that it contains fields even if they are zeroed
		for _, field := range []string{
			"type", "lastBytesSent", "lastBytesReceived", "lastRetransmits",
			"netNS", "family", "direction", "pid",
		} {
			assert.Contains(res.Conns[0], field)
		}
	})

	// protobufs evaluate empty maps as nil
	out.CompilationTelemetryByAsset = nil

	t.Run("requesting application/protobuf serialization", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal("application/protobuf", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		assert.Equal(out, result)
	})
}

func TestFormatHTTPStatsByPath(t *testing.T) {
	var httpReqStats http.RequestStats
	httpReqStats.AddRequest(100, 12.5)
	httpReqStats.AddRequest(100, 12.5)
	httpReqStats.AddRequest(405, 3.5)
	httpReqStats.AddRequest(405, 3.5)

	// Verify the latency data is correct prior to serialization
	latencies := httpReqStats[model.HTTPResponseStatus_Info].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 12.5)

	latencies = httpReqStats[model.HTTPResponseStatus_ClientErr].Latencies
	assert.Equal(t, 2.0, latencies.GetCount())
	verifyQuantile(t, latencies, 0.5, 3.5)

	key := http.NewKey(
		util.AddressFromString("10.1.1.1"),
		util.AddressFromString("10.2.2.2"),
		1000,
		9000,
		"/testpath",
		http.MethodGet,
	)
	statsByKey := map[http.Key]http.RequestStats{
		key: httpReqStats,
	}
	formattedStats := FormatHTTPStats(statsByKey)

	// Now path will be nested in the map
	key.Path = ""
	key.Method = http.MethodUnknown

	endpointAggregations := formattedStats[key].EndpointAggregations
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

func TestHTTPSerializationWithLocalhostTraffic(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	var httpReqStats http.RequestStats
	in := &network.Connections{
		Conns: []network.ConnectionStats{
			{
				Source: localhost,
				Dest:   localhost,
				SPort:  clientPort,
				DPort:  serverPort,
			},
			{
				Source: localhost,
				Dest:   localhost,
				SPort:  serverPort,
				DPort:  clientPort,
			},
		},
		HTTP: map[http.Key]http.RequestStats{
			http.NewKey(
				localhost,
				localhost,
				clientPort,
				serverPort,
				"/testpath",
				http.MethodGet,
			): httpReqStats,
		},
	}

	httpOut := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:   "/testpath",
				Method: model.HTTPMethod_Get,
				StatsByResponseStatus: []*model.HTTPStats_Data{
					{Count: 0, Latencies: nil},
					{Count: 0, Latencies: nil},
					{Count: 0, Latencies: nil},
					{Count: 0, Latencies: nil},
					{Count: 0, Latencies: nil},
				},
			},
		},
	}

	httpOutBlob, err := proto.Marshal(httpOut)
	require.NoError(t, err)

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Raddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				HttpAggregations: httpOutBlob,
				RouteIdx:         -1,
			},
			{
				Laddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				HttpAggregations: httpOutBlob,
				RouteIdx:         -1,
			},
		},
	}

	marshaler := GetMarshaler("application/protobuf")
	blob, err := marshaler.Marshal(in)
	require.NoError(t, err)

	unmarshaler := GetUnmarshaler("application/protobuf")
	result, err := unmarshaler.Unmarshal(blob)
	require.NoError(t, err)

	assert.Equal(t, out, result)
}

func unmarshalSketch(t *testing.T, bytes []byte) *ddsketch.DDSketch {
	var sketchPb sketchpb.DDSketch
	err := proto.Unmarshal(bytes, &sketchPb)
	assert.Nil(t, err)

	var sketch *ddsketch.DDSketch
	ret, err := sketch.FromProto(&sketchPb)
	assert.Nil(t, err)

	return ret
}

func verifyQuantile(t *testing.T, sketch *ddsketch.DDSketch, q float64, expectedValue float64) {
	val, err := sketch.GetValueAtQuantile(q)
	assert.Nil(t, err)

	acceptableError := expectedValue * http.RelativeAccuracy
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
