package encoding

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/sketches-go/ddsketch"
	"github.com/DataDog/sketches-go/ddsketch/pb/sketchpb"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var originalConfig = config.Datadog

func restoreGlobalConfig() {
	config.Datadog = originalConfig
}

func newConfig() {
	config.Datadog = config.NewConfig("datadog", "DD", strings.NewReplacer(".", "_"))
	config.InitConfig(config.Datadog)
}
func getExpectedConnections(encodedWithQueryType bool, httpOutBlob []byte) *model.Connections {
	var dnsByDomain map[int32]*model.DNSStats
	var dnsByDomainByQuerytype map[int32]*model.DNSStatsByQueryType

	if encodedWithQueryType {
		dnsByDomain = map[int32]*model.DNSStats{}
		dnsByDomainByQuerytype = map[int32]*model.DNSStatsByQueryType{
			0: {
				DnsStatsByQueryType: map[int32]*model.DNSStats{
					int32(network.DNSTypeA): {
						DnsTimeouts:          0,
						DnsSuccessLatencySum: 0,
						DnsFailureLatencySum: 0,
						DnsCountByRcode:      map[uint32]uint32{0: 1},
					},
				},
			},
		}
	} else {
		dnsByDomainByQuerytype = map[int32]*model.DNSStatsByQueryType{}
		dnsByDomain = map[int32]*model.DNSStats{
			0: {
				DnsTimeouts:          0,
				DnsSuccessLatencySum: 0,
				DnsFailureLatencySum: 0,
				DnsCountByRcode:      map[uint32]uint32{0: 1},
			},
		}
	}

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

				DnsCountByRcode:             map[uint32]uint32{0: 1},
				DnsStatsByDomain:            dnsByDomain,
				DnsStatsByDomainByQueryType: dnsByDomainByQuerytype,
				RouteIdx:                    0,
				HttpAggregations:            httpOutBlob,
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
	return out
}
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
				DNSStatsByDomainByQueryType: map[string]map[network.QueryType]network.DNSStats{
					"foo.com": {
						network.DNSTypeA: {
							DNSTimeouts:          0,
							DNSSuccessLatencySum: 0,
							DNSFailureLatencySum: 0,
							DNSCountByRcode:      map[uint32]uint32{0: 1},
						},
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

	t.Run("requesting application/json serialization (no query types)", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()
		out := getExpectedConnections(false, httpOutBlob)
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
	t.Run("requesting application/json serialization (with query types)", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()
		config.Datadog.Set("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)
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
		newConfig()
		defer restoreGlobalConfig()
		out := getExpectedConnections(false, httpOutBlob)
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
		newConfig()
		defer restoreGlobalConfig()
		out := getExpectedConnections(false, httpOutBlob)

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

	t.Run("requesting application/protobuf serialization (no query types)", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()
		out := getExpectedConnections(false, httpOutBlob)
		// protobufs evaluate empty maps as nil
		out.CompilationTelemetryByAsset = nil

		assert := assert.New(t)
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal("application/protobuf", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		// there seems to be a bug in protobuf maps with integer keys; it will
		// not round-trip an empty map properly.  Temporarily hack around this problem
		if result.Conns[0].DnsStatsByDomain == nil {
			result.Conns[0].DnsStatsByDomain = map[int32]*model.DNSStats{}
		}
		if result.Conns[0].DnsStatsByDomainByQueryType == nil {
			result.Conns[0].DnsStatsByDomainByQueryType = map[int32]*model.DNSStatsByQueryType{}
		}

		assert.Equal(out, result)
	})
	t.Run("requesting application/protobuf serialization (with query types)", func(t *testing.T) {
		newConfig()
		defer restoreGlobalConfig()
		config.Datadog.Set("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)
		// protobufs evaluate empty maps as nil
		out.CompilationTelemetryByAsset = nil

		assert := assert.New(t)
		marshaler := GetMarshaler("application/protobuf")
		assert.Equal("application/protobuf", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		// there seems to be a bug in protobuf maps with integer keys; it will
		// not round-trip an empty map properly.  Temporarily hack around this problem
		if result.Conns[0].DnsStatsByDomain == nil {
			result.Conns[0].DnsStatsByDomain = map[int32]*model.DNSStats{}
		}

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

func TestPooledObjectGarbageRegression(t *testing.T) {
	// This test ensures that no garbage data is accidentally
	// left on pooled Connection objects used during serialization
	httpKey := http.NewKey(
		util.AddressFromString("10.0.15.1"),
		util.AddressFromString("172.217.10.45"),
		60000,
		8080,
		"",
		http.MethodGet,
	)

	in := &network.Connections{
		Conns: []network.ConnectionStats{
			{
				Source: util.AddressFromString("10.0.15.1"),
				SPort:  uint16(60000),
				Dest:   util.AddressFromString("172.217.10.45"),
				DPort:  uint16(8080),
			},
		},
	}

	encodeAndDecodeHTTP := func(c *network.Connections) *model.HTTPAggregations {
		marshaler := GetMarshaler("application/protobuf")
		blob, err := marshaler.Marshal(c)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		httpBlob := result.Conns[0].HttpAggregations
		if httpBlob == nil {
			return nil
		}

		httpOut := new(model.HTTPAggregations)
		err = proto.Unmarshal(httpBlob, httpOut)
		require.NoError(t, err)
		return httpOut
	}

	// Let's alternate between payloads with and without HTTP data
	for i := 0; i < 1000; i++ {
		if (i % 2) == 0 {
			httpKey.Path = fmt.Sprintf("/path-%d", i)
			in.HTTP = map[http.Key]http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path, out.EndpointAggregations[0].Path)
		} else {
			// No HTTP data in this payload, so we should never get HTTP data back after the serialization
			in.HTTP = nil
			out := encodeAndDecodeHTTP(in)
			require.Nil(t, out, "expected a nil object, but got garbage")
		}
	}
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

	acceptableError := expectedValue * sketch.IndexMapping.RelativeAccuracy()
	assert.True(t, val >= expectedValue-acceptableError)
	assert.True(t, val <= expectedValue+acceptableError)
}
