// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type connTag = uint64

// ConnTag constant must be the same for all platform
const (
	tagGnuTLS  connTag = 1 // netebpf.GnuTLS
	tagOpenSSL connTag = 2 // netebpf.OpenSSL
)

func newConfig(t *testing.T) {
	originalConfig := config.SystemProbe
	t.Cleanup(func() {
		config.SystemProbe = originalConfig
	})
	config.SystemProbe = config.NewConfig("system-probe", "DD", strings.NewReplacer(".", "_"))
	config.InitSystemProbeConfig(config.SystemProbe)
}

func getExpectedConnections(encodedWithQueryType bool, httpOutBlob []byte) *model.Connections {
	var dnsByDomain map[int32]*model.DNSStats
	var dnsByDomainByQuerytype map[int32]*model.DNSStatsByQueryType

	if encodedWithQueryType {
		dnsByDomainByQuerytype = map[int32]*model.DNSStatsByQueryType{
			0: {
				DnsStatsByQueryType: map[int32]*model.DNSStats{
					int32(dns.TypeA): {
						DnsTimeouts:          0,
						DnsSuccessLatencySum: 0,
						DnsFailureLatencySum: 0,
						DnsCountByRcode:      map[uint32]uint32{0: 1},
					},
				},
			},
		}
	} else {
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
					ReplSrcPort: int32(40000),
					ReplDstPort: int32(80),
				},

				Type:      model.ConnectionType_udp,
				Family:    model.ConnectionFamily_v6,
				Direction: model.ConnectionDirection_local,

				RouteIdx:         0,
				HttpAggregations: httpOutBlob,
				Protocol: &model.ProtocolStack{
					Stack: []model.ProtocolType{model.ProtocolType_protocolHTTP},
				},
			},
			{
				Laddr: &model.Addr{Ip: "10.1.1.1", Port: int32(1000)},
				Raddr: &model.Addr{Ip: "8.8.8.8", Port: int32(53)},

				Type:      model.ConnectionType_udp,
				Family:    model.ConnectionFamily_v6,
				Direction: model.ConnectionDirection_local,

				DnsCountByRcode:             map[uint32]uint32{0: 1},
				DnsStatsByDomain:            dnsByDomain,
				DnsStatsByDomainByQueryType: dnsByDomainByQuerytype,
				DnsSuccessfulResponses:      1, // TODO: verify why this was needed

				RouteIdx: -1,
				Protocol: &model.ProtocolStack{
					Stack: []model.ProtocolType{
						model.ProtocolType_protocolHTTP2,
					},
				},
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
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
		},
		Tags: network.GetStaticTags(1),
	}
	if runtime.GOOS == "linux" {
		out.Conns[1].Tags = []uint32{0}
		out.Conns[1].TagsChecksum = uint32(3241915907)
		out.Conns[1].Protocol.Stack = append([]model.ProtocolType{model.ProtocolType_protocolTLS}, out.Conns[1].Protocol.Stack...)

	}
	return out
}

func TestSerialization(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testSerialization(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testSerialization(t, false)
	})
}

func testSerialization(t *testing.T, aggregateByStatusCode bool) {
	httpReqStats := http.NewRequestStats(aggregateByStatusCode)
	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("10.2.2.2"),
					Monotonic: network.StatCounters{
						SentBytes:   1,
						RecvBytes:   100,
						Retransmits: 201,
					},
					Last: network.StatCounters{
						SentBytes:      2,
						RecvBytes:      101,
						TCPEstablished: 1,
						TCPClosed:      1,
						Retransmits:    201,
					},
					LastUpdateEpoch: 50,
					Pid:             6000,
					NetNS:           7,
					SPort:           1000,
					DPort:           9000,
					IPTranslation: &network.IPTranslation{
						ReplSrcIP:   util.AddressFromString("20.1.1.1"),
						ReplDstIP:   util.AddressFromString("20.1.1.1"),
						ReplSrcPort: 40000,
						ReplDstPort: 80,
					},

					Type:      network.UDP,
					Family:    network.AFINET6,
					Direction: network.LOCAL,
					Via: &network.Via{
						Subnet: network.Subnet{
							Alias: "subnet-foo",
						},
					},
					Protocol: network.ProtocolHTTP,
				},
				{
					Source:     util.AddressFromString("10.1.1.1"),
					Dest:       util.AddressFromString("8.8.8.8"),
					SPort:      1000,
					DPort:      53,
					Type:       network.UDP,
					Family:     network.AFINET6,
					Direction:  network.LOCAL,
					StaticTags: uint64(1),
					Protocol:   network.ProtocolHTTP2,
				},
			},
		},
		DNS: map[util.Address][]dns.Hostname{
			util.AddressFromString("172.217.12.145"): {dns.ToHostname("golang.org")},
		},
		DNSStats: dns.StatsByKeyByNameByType{
			dns.Key{
				ClientIP:   util.AddressFromString("10.1.1.1"),
				ServerIP:   util.AddressFromString("8.8.8.8"),
				ClientPort: uint16(1000),
				Protocol:   syscall.IPPROTO_UDP,
			}: map[dns.Hostname]map[dns.QueryType]dns.Stats{
				dns.ToHostname("foo.com"): {
					dns.TypeA: {
						Timeouts:          0,
						SuccessLatencySum: 0,
						FailureLatencySum: 0,
						CountByRcode:      map[uint32]uint32{0: 1},
					},
				},
			},
		},
		HTTP: map[http.Key]*http.RequestStats{
			http.NewKey(
				util.AddressFromString("20.1.1.1"),
				util.AddressFromString("20.1.1.1"),
				40000,
				80,
				"/testpath",
				true,
				http.MethodGet,
			): httpReqStats,
		},
	}

	httpOut := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:              "/testpath",
				Method:            model.HTTPMethod_Get,
				FullPath:          true,
				StatsByStatusCode: make(map[int32]*model.HTTPStats_Data),
			},
		},
	}

	httpOutBlob, err := proto.Marshal(httpOut)
	require.NoError(t, err)

	t.Run("requesting application/json serialization (no query types)", func(t *testing.T) {
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assert.Equal(out, result)
	})
	t.Run("requesting application/json serialization (with query types)", func(t *testing.T) {
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
		config.SystemProbe.Set("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		blob, err := marshaler.Marshal(in)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assert.Equal(out, result)
	})

	t.Run("requesting empty serialization", func(t *testing.T) {
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
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

		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assert.Equal(out, result)
	})

	t.Run("requesting unsupported serialization format", func(t *testing.T) {
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
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

		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assert.Equal(out, result)
	})

	t.Run("render default values with application/json", func(t *testing.T) {
		assert := assert.New(t)
		marshaler := GetMarshaler("application/json")
		assert.Equal("application/json", marshaler.ContentType())

		// Empty connection batch
		blob, err := marshaler.Marshal(&network.Connections{
			BufferedData: network.BufferedData{
				Conns: []network.ConnectionStats{{}},
			},
		})
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
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)

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
	t.Run("requesting application/protobuf serialization (with query types)", func(t *testing.T) {
		newConfig(t)
		config.SystemProbe.Set("system_probe_config.collect_dns_domains", false)
		config.SystemProbe.Set("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)

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

func TestHTTPSerializationWithLocalhostTraffic(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testHTTPSerializationWithLocalhostTraffic(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testHTTPSerializationWithLocalhostTraffic(t, false)
	})
}

func testHTTPSerializationWithLocalhostTraffic(t *testing.T, aggregateByStatusCode bool) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	httpReqStats := http.NewRequestStats(aggregateByStatusCode)
	in := &network.Connections{
		BufferedData: network.BufferedData{
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
		},
		HTTP: map[http.Key]*http.RequestStats{
			http.NewKey(
				localhost,
				localhost,
				clientPort,
				serverPort,
				"/testpath",
				true,
				http.MethodGet,
			): httpReqStats,
		},
	}

	httpOut := &model.HTTPAggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:              "/testpath",
				Method:            model.HTTPMethod_Get,
				FullPath:          true,
				StatsByStatusCode: make(map[int32]*model.HTTPStats_Data),
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
				Protocol:         formatProtocol(network.ProtocolUnknown, 0),
			},
			{
				Laddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				HttpAggregations: httpOutBlob,
				RouteIdx:         -1,
				Protocol:         formatProtocol(network.ProtocolUnknown, 0),
			},
		},
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
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

func TestHTTP2SerializationWithLocalhostTraffic(t *testing.T) {
	t.Run("status code", func(t *testing.T) {
		testHTTP2SerializationWithLocalhostTraffic(t, true)
	})
	t.Run("status class", func(t *testing.T) {
		testHTTP2SerializationWithLocalhostTraffic(t, false)
	})
}

func testHTTP2SerializationWithLocalhostTraffic(t *testing.T, aggregateByStatusCode bool) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	http2ReqStats := http.NewRequestStats(aggregateByStatusCode)
	in := &network.Connections{
		BufferedData: network.BufferedData{
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
		},
		HTTP2: map[http.Key]*http.RequestStats{
			http.NewKey(
				localhost,
				localhost,
				clientPort,
				serverPort,
				"/testpath",
				true,
				http.MethodPost,
			): http2ReqStats,
		},
	}

	http2Out := &model.HTTP2Aggregations{
		EndpointAggregations: []*model.HTTPStats{
			{
				Path:              "/testpath",
				Method:            model.HTTPMethod_Post,
				FullPath:          true,
				StatsByStatusCode: make(map[int32]*model.HTTPStats_Data),
			},
		},
	}

	http2OutBlob, err := proto.Marshal(http2Out)
	require.NoError(t, err)

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Raddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Http2Aggregations: http2OutBlob,
				RouteIdx:          -1,
				Protocol:          formatProtocol(network.ProtocolUnknown, 0),
			},
			{
				Laddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Http2Aggregations: http2OutBlob,
				RouteIdx:          -1,
				Protocol:          formatProtocol(network.ProtocolUnknown, 0),
			},
		},
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
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
		true,
		http.MethodGet,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: util.AddressFromString("10.0.15.1"),
					SPort:  uint16(60000),
					Dest:   util.AddressFromString("172.217.10.45"),
					DPort:  uint16(8080),
				},
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
			httpKey.Path = http.Path{
				Content:  fmt.Sprintf("/path-%d", i),
				FullPath: true,
			}
			in.HTTP = map[http.Key]*http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path.Content, out.EndpointAggregations[0].Path)
		} else {
			// No HTTP data in this payload, so we should never get HTTP data back after the serialization
			in.HTTP = nil
			out := encodeAndDecodeHTTP(in)
			require.Nil(t, out, "expected a nil object, but got garbage")
		}
	}
}

func TestPooledHTTP2ObjectGarbageRegression(t *testing.T) {
	// This test ensures that no garbage data is accidentally
	// left on pooled Connection objects used during serialization
	httpKey := http.NewKey(
		util.AddressFromString("10.0.15.1"),
		util.AddressFromString("172.217.10.45"),
		60000,
		8080,
		"",
		true,
		http.MethodGet,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{
					Source: util.AddressFromString("10.0.15.1"),
					SPort:  uint16(60000),
					Dest:   util.AddressFromString("172.217.10.45"),
					DPort:  uint16(8080),
				},
			},
		},
	}

	encodeAndDecodeHTTP2 := func(c *network.Connections) *model.HTTP2Aggregations {
		marshaler := GetMarshaler("application/protobuf")
		blob, err := marshaler.Marshal(c)
		require.NoError(t, err)

		unmarshaler := GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blob)
		require.NoError(t, err)

		http2Blob := result.Conns[0].Http2Aggregations
		if http2Blob == nil {
			return nil
		}

		http2Out := new(model.HTTP2Aggregations)
		err = proto.Unmarshal(http2Blob, http2Out)
		require.NoError(t, err)
		return http2Out
	}

	// Let's alternate between payloads with and without HTTP2 data
	for i := 0; i < 1000; i++ {
		if (i % 2) == 0 {
			httpKey.Path = http.Path{
				Content:  fmt.Sprintf("/path-%d", i),
				FullPath: true,
			}
			in.HTTP2 = map[http.Key]*http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP2(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path.Content, out.EndpointAggregations[0].Path)
		} else {
			// No HTTP2 data in this payload, so we should never get HTTP2 data back after the serialization
			in.HTTP2 = nil
			out := encodeAndDecodeHTTP2(in)
			require.Nil(t, out, "expected a nil object, but got garbage")
		}
	}
}
