// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package encoding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"runtime"
	"sort"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	model "github.com/DataDog/agent-payload/v5/process"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/dns"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

type connTag = uint64

// ConnTag constant must be the same for all platform
const (
	tagGnuTLS  connTag = 0x01 // network.ConnTagGnuTLS
	tagOpenSSL connTag = 0x02 // network.ConnTagOpenSSL
	tagTLS     connTag = 0x8  // network.ConnTagTLS
)

func getBlobWriter(t *testing.T, assert *assert.Assertions, in *network.Connections, marshalerType string) *bytes.Buffer {
	marshaler := marshal.GetMarshaler(marshalerType)
	assert.Equal(marshalerType, marshaler.ContentType())
	blobWriter := bytes.NewBuffer(nil)
	connectionsModeler := marshal.NewConnectionsModeler(in)
	defer connectionsModeler.Close()
	err := marshaler.Marshal(in, blobWriter, connectionsModeler)
	require.NoError(t, err)

	return blobWriter
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

				Type:      model.ConnectionType_tcp,
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
				TcpFailuresByErrCode:        map[uint32]uint32{110: 1},
				RouteIdx:                    -1,
				Protocol: &model.ProtocolStack{
					Stack: []model.ProtocolType{model.ProtocolType_protocolTLS, model.ProtocolType_protocolHTTP2},
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
		Tags: network.GetStaticTags(tagOpenSSL | tagTLS),
	}
	// fixup Protocol stack as on windows or macos
	// we don't have tags mechanism inserting TLS protocol on protocol stack
	if runtime.GOOS != "linux" {
		for _, c := range out.Conns {
			stack := []model.ProtocolType{}
			for _, p := range c.Protocol.Stack {
				if p == model.ProtocolType_protocolTLS {
					continue
				}
				stack = append(stack, p)
			}
			c.Protocol.Stack = stack
		}
	}
	sort.Strings(out.Tags)
	if runtime.GOOS == "linux" {
		out.Conns[1].Tags = []uint32{0, 1}
		out.Conns[1].TagsChecksum = uint32(3359960845)
	}
	if runtime.GOOS == "windows" {
		/*
		 * on Windows, there are separate http transactions for
		 * each side of the connection.  And they're kept separate,
		 * and keyed separately.  Address this condition until the
		 * platforms are resynced
		 *
		 * Also on windows, we do not use the NAT translation.  There
		 * is an artifact of the NAT translation that results in
		 * being unable to match the connectoin at this time, due
		 * to the above.  Remove the nat translation, so that we're
		 * still testing the rest of the encoding functions.
		 *
		 * there is the corresponding change required in
		 * testSerialization() below
		 */
		out.Conns[0].IpTranslation = nil
	}
	return out
}

func TestSerialization(t *testing.T) {
	httpReqStats := http.NewRequestStats()
	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("10.2.2.2"),
					Pid:    6000,
					NetNS:  7,
					SPort:  1000,
					DPort:  9000,
					Type:   network.TCP,
					Family: network.AFINET6,
				},
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

					IPTranslation: &network.IPTranslation{
						ReplSrcIP:   util.AddressFromString("20.1.1.1"),
						ReplDstIP:   util.AddressFromString("20.1.1.1"),
						ReplSrcPort: 40000,
						ReplDstPort: 80,
					},

					Direction: network.LOCAL,
					Via: &network.Via{
						Subnet: network.Subnet{
							Alias: "subnet-foo",
						},
					},
					ProtocolStack: protocols.Stack{Application: protocols.HTTP},
				},
				{ConnectionTuple: network.ConnectionTuple{
					Source: util.AddressFromString("10.1.1.1"),
					Dest:   util.AddressFromString("8.8.8.8"),
					SPort:  1000,
					DPort:  53,
					Type:   network.UDP,
					Family: network.AFINET6,
				},
					Direction:     network.LOCAL,
					StaticTags:    tagOpenSSL | tagTLS,
					ProtocolStack: protocols.Stack{Application: protocols.HTTP2},
					DNSStats: map[dns.Hostname]map[dns.QueryType]dns.Stats{
						dns.ToHostname("foo.com"): {
							dns.TypeA: {
								Timeouts:          0,
								SuccessLatencySum: 0,
								FailureLatencySum: 0,
								CountByRcode:      map[uint32]uint32{0: 1},
							},
						},
					},
					TCPFailures: map[uint32]uint32{
						110: 1,
					},
				},
			},
		},
		DNS: map[util.Address][]dns.Hostname{
			util.AddressFromString("172.217.12.145"): {dns.ToHostname("golang.org")},
		},
		HTTP: map[http.Key]*http.RequestStats{
			http.NewKey(
				util.AddressFromString("20.1.1.1"),
				util.AddressFromString("20.1.1.1"),
				40000,
				80,
				[]byte("/testpath"),
				true,
				http.MethodGet,
			): httpReqStats,
		},
	}

	if runtime.GOOS == "windows" {
		/*
		 * on Windows, there are separate http transactions for
		 * each side of the connection.  And they're kept separate,
		 * and keyed separately.  Address this condition until the
		 * platforms are resynced
		 *
		 * Also on windows, we do not use the NAT translation.  There
		 * is an artifact of the NAT translation that results in
		 * being unable to match the connectoin at this time, due
		 * to the above.  Remove the nat translation, so that we're
		 * still testing the rest of the encoding functions.
		 *
		 * there is a corresponding change in the above helper function
		 * getExpectedConnections()
		 */
		in.BufferedData.Conns[0].IPTranslation = nil
		in.HTTP = map[http.Key]*http.RequestStats{
			http.NewKey(
				util.AddressFromString("10.1.1.1"),
				util.AddressFromString("10.2.2.2"),
				1000,
				9000,
				[]byte("/testpath"),
				true,
				http.MethodGet,
			): httpReqStats,
		}
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
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)
		assert := assert.New(t)
		blobWriter := getBlobWriter(t, assert, in, "application/json")

		unmarshaler := unmarshal.GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)

		sort.Strings(result.Tags)
		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assertConnsEqual(t, out, result)
	})

	t.Run("requesting application/json serialization (with query types)", func(t *testing.T) {
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		pkgconfigsetup.SystemProbe().SetWithoutSource("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)
		assert := assert.New(t)

		blobWriter := getBlobWriter(t, assert, in, "application/json")

		unmarshaler := unmarshal.GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)

		sort.Strings(result.Tags)
		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assertConnsEqual(t, out, result)
	})

	t.Run("requesting empty serialization", func(t *testing.T) {
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)
		assert := assert.New(t)

		marshaler := marshal.GetMarshaler("")
		// in case we request empty serialization type, default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blobWriter := bytes.NewBuffer(nil)
		connectionsModeler := marshal.NewConnectionsModeler(in)
		defer connectionsModeler.Close()
		err := marshaler.Marshal(in, blobWriter, connectionsModeler)
		require.NoError(t, err)

		unmarshaler := unmarshal.GetUnmarshaler("")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)

		sort.Strings(result.Tags)
		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assertConnsEqual(t, out, result)
	})

	t.Run("requesting unsupported serialization format", func(t *testing.T) {
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)

		assert := assert.New(t)
		marshaler := marshal.GetMarshaler("application/whatever")

		// In case we request an unsupported serialization type, we default to application/json
		assert.Equal("application/json", marshaler.ContentType())

		blobWriter := bytes.NewBuffer(nil)
		connectionsModeler := marshal.NewConnectionsModeler(in)
		defer connectionsModeler.Close()
		err := marshaler.Marshal(in, blobWriter, connectionsModeler)
		require.NoError(t, err)

		unmarshaler := unmarshal.GetUnmarshaler("application/json")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)

		sort.Strings(result.Tags)
		// fixup: json marshaler encode nil slice as empty
		result.Conns[0].Tags = nil
		if runtime.GOOS != "linux" {
			result.Conns[1].Tags = nil
			result.Tags = nil
		}
		result.PrebuiltEBPFAssets = nil
		assertConnsEqual(t, out, result)
	})

	t.Run("render default values with application/json", func(t *testing.T) {
		assert := assert.New(t)

		// Empty connection batch
		blobWriter := getBlobWriter(t, assert, &network.Connections{
			BufferedData: network.BufferedData{
				Conns: []network.ConnectionStats{{}},
			}}, "application/json")

		res := struct {
			Conns []map[string]interface{} `json:"conns"`
		}{}
		require.NoError(t, json.Unmarshal(blobWriter.Bytes(), &res))

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
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		out := getExpectedConnections(false, httpOutBlob)

		assert := assert.New(t)

		blobWriter := getBlobWriter(t, assert, in, "application/protobuf")

		unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)
		sort.Strings(result.Tags)

		assertConnsEqual(t, out, result)
	})
	t.Run("requesting application/protobuf serialization (with query types)", func(t *testing.T) {
		configmock.NewSystemProbe(t)
		pkgconfigsetup.SystemProbe().SetWithoutSource("system_probe_config.collect_dns_domains", false)
		pkgconfigsetup.SystemProbe().SetWithoutSource("network_config.enable_dns_by_querytype", true)
		out := getExpectedConnections(true, httpOutBlob)

		assert := assert.New(t)
		blobWriter := getBlobWriter(t, assert, in, "application/protobuf")

		unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
		require.NoError(t, err)
		sort.Strings(result.Tags)

		assertConnsEqual(t, out, result)
	})
}

func TestHTTPSerializationWithLocalhostTraffic(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	httpReqStats := http.NewRequestStats()
	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: localhost,
					Dest:   localhost,
					SPort:  clientPort,
					DPort:  serverPort,
				}},
				{ConnectionTuple: network.ConnectionTuple{
					Source: localhost,
					Dest:   localhost,
					SPort:  serverPort,
					DPort:  clientPort,
				}},
			},
		},
		HTTP: map[http.Key]*http.RequestStats{
			http.NewKey(
				localhost,
				localhost,
				clientPort,
				serverPort,
				[]byte("/testpath"),
				true,
				http.MethodGet,
			): httpReqStats,
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
			localhost,
			localhost,
			serverPort,
			clientPort,
			[]byte("/testpath"),
			true,
			http.MethodGet,
		)

		in.HTTP[httpKeyWin] = httpReqStats
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
				Protocol:         marshal.FormatProtocolStack(protocols.Stack{}, 0),
			},
			{
				Laddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:            &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				HttpAggregations: httpOutBlob,
				RouteIdx:         -1,
				Protocol:         marshal.FormatProtocolStack(protocols.Stack{}, 0),
			},
		},
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
		},
	}

	blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

	unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
	result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
	require.NoError(t, err)
	assertConnsEqual(t, out, result)
}

func assertConnsEqual(t *testing.T, expected, actual *model.Connections) {
	require.Equal(t, len(expected.Conns), len(actual.Conns), "expected both model.Connections to have the same number of connections")

	for i := 0; i < len(actual.Conns); i++ {
		expectedRawHTTP := expected.Conns[i].HttpAggregations
		actualRawHTTP := actual.Conns[i].HttpAggregations

		if len(expectedRawHTTP) == 0 && len(actualRawHTTP) != 0 {
			t.Fatalf("expected connection %d to have no HTTP, but got %v", i, actualRawHTTP)
		}
		if len(expectedRawHTTP) != 0 && len(actualRawHTTP) == 0 {
			t.Fatalf("expected connection %d to have HTTP data, but got none", i)
		}

		// the expected HTTPAggregations are encoded with  gogoproto, and the actual HTTPAggregations are encoded with gostreamer.
		// thus they will not be byte-for-byte equal.
		// the workaround is to check for protobuf equality, and then set actual.Conns[i] == expected.Conns[i]
		// so actual.Conns and expected.Conns can be compared.
		var expectedHTTP, actualHTTP model.HTTPAggregations
		require.NoError(t, proto.Unmarshal(expectedRawHTTP, &expectedHTTP))
		require.NoError(t, proto.Unmarshal(actualRawHTTP, &actualHTTP))
		require.Equalf(t, expectedHTTP, actualHTTP, "HTTP connection %d was not equal", i)
		actual.Conns[i].HttpAggregations = expected.Conns[i].HttpAggregations
	}

	assert.Equal(t, expected, actual)

}

func assertConnsEqualHTTP2(t *testing.T, expected, actual *model.Connections) {
	require.Equal(t, len(expected.Conns), len(actual.Conns), "expected both model.Connections to have the same number of connections")

	for i := 0; i < len(actual.Conns); i++ {
		expectedRawHTTP2 := expected.Conns[i].Http2Aggregations
		actualRawHTTP2 := actual.Conns[i].Http2Aggregations

		if len(expectedRawHTTP2) == 0 && len(actualRawHTTP2) != 0 {
			t.Fatalf("expected connection %d to have no HTTP2, but got %v", i, actualRawHTTP2)
		}
		if len(expectedRawHTTP2) != 0 && len(actualRawHTTP2) == 0 {
			t.Fatalf("expected connection %d to have HTTP2 data, but got none", i)
		}

		// the expected HTTPAggregations are encoded with  gogoproto, and the actual HTTPAggregations are encoded with gostreamer.
		// thus they will not be byte-for-byte equal.
		// the workaround is to check for protobuf equality, and then set actual.Conns[i] == expected.Conns[i]
		// so actual.Conns and expected.Conns can be compared.
		var expectedHTTP2, actualHTTP2 model.HTTP2Aggregations
		require.NoError(t, proto.Unmarshal(expectedRawHTTP2, &expectedHTTP2))
		require.NoError(t, proto.Unmarshal(actualRawHTTP2, &actualHTTP2))
		require.Equalf(t, expectedHTTP2, actualHTTP2, "HTTP2 connection %d was not equal", i)
		actual.Conns[i].Http2Aggregations = expected.Conns[i].Http2Aggregations
	}

	assert.Equal(t, expected, actual)

}

func TestHTTP2SerializationWithLocalhostTraffic(t *testing.T) {
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	http2ReqStats := http.NewRequestStats()
	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: localhost,
					Dest:   localhost,
					SPort:  clientPort,
					DPort:  serverPort,
				}},
				{ConnectionTuple: network.ConnectionTuple{
					Source: localhost,
					Dest:   localhost,
					SPort:  serverPort,
					DPort:  clientPort,
				}},
			},
		},
		HTTP2: map[http.Key]*http.RequestStats{
			http.NewKey(
				localhost,
				localhost,
				clientPort,
				serverPort,
				[]byte("/testpath"),
				true,
				http.MethodPost,
			): http2ReqStats,
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
			localhost,
			localhost,
			serverPort,
			clientPort,
			[]byte("/testpath"),
			true,
			http.MethodPost,
		)

		in.HTTP2[httpKeyWin] = http2ReqStats
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
				Protocol:          marshal.FormatProtocolStack(protocols.Stack{}, 0),
			},
			{
				Laddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Http2Aggregations: http2OutBlob,
				RouteIdx:          -1,
				Protocol:          marshal.FormatProtocolStack(protocols.Stack{}, 0),
			},
		},
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
		},
	}
	blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

	unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
	result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
	require.NoError(t, err)

	assertConnsEqualHTTP2(t, out, result)
}

func TestPooledObjectGarbageRegression(t *testing.T) {
	// This test ensures that no garbage data is accidentally
	// left on pooled Connection objects used during serialization
	httpKey := http.NewKey(
		util.AddressFromString("10.0.15.1"),
		util.AddressFromString("172.217.10.45"),
		60000,
		8080,
		nil,
		true,
		http.MethodGet,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: util.AddressFromString("10.0.15.1"),
					SPort:  uint16(60000),
					Dest:   util.AddressFromString("172.217.10.45"),
					DPort:  uint16(8080),
				}},
			},
		},
	}

	encodeAndDecodeHTTP := func(*network.Connections) *model.HTTPAggregations {
		blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

		unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
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
				Content:  http.Interner.GetString(fmt.Sprintf("/path-%d", i)),
				FullPath: true,
			}
			in.HTTP = map[http.Key]*http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path.Content.Get(), out.EndpointAggregations[0].Path)
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
		nil,
		true,
		http.MethodGet,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: []network.ConnectionStats{
				{ConnectionTuple: network.ConnectionTuple{
					Source: util.AddressFromString("10.0.15.1"),
					SPort:  uint16(60000),
					Dest:   util.AddressFromString("172.217.10.45"),
					DPort:  uint16(8080),
				}},
			},
		},
	}

	encodeAndDecodeHTTP2 := func(*network.Connections) *model.HTTP2Aggregations {
		blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

		unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
		result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
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
				Content:  http.Interner.GetString(fmt.Sprintf("/path-%d", i)),
				FullPath: true,
			}
			in.HTTP2 = map[http.Key]*http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP2(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path.Content.Get(), out.EndpointAggregations[0].Path)
		} else {
			// No HTTP2 data in this payload, so we should never get HTTP2 data back after the serialization
			in.HTTP2 = nil
			out := encodeAndDecodeHTTP2(in)
			require.Nil(t, out, "expected a nil object, but got garbage")
		}
	}
}

func TestUSMPayloadTelemetry(t *testing.T) {
	telemetry.Clear()
	t.Cleanup(telemetry.Clear)

	// Set metric present in the payload telemetry list to an arbitrary value
	m1 := telemetry.NewCounter("usm.http.total_hits", telemetry.OptPayloadTelemetry)
	m1.Add(10)
	require.Contains(t, network.USMPayloadTelemetry, network.ConnTelemetryType(m1.Name()))

	// Add another metric that is not present in the allowed list
	m2 := telemetry.NewCounter("foobar", telemetry.OptPayloadTelemetry)
	m2.Add(50)
	require.NotContains(t, network.USMPayloadTelemetry, network.ConnTelemetryType(m2.Name()))

	// Perform a marshal/unmarshal cycle
	in := new(network.Connections)
	blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

	unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
	result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
	require.NoError(t, err)

	// Assert that the correct metric is present in the emitted payload
	payloadTelemetry := result.ConnTelemetryMap
	assert.Equal(t, int64(10), payloadTelemetry["usm.http.total_hits"])
	assert.NotContains(t, payloadTelemetry, "foobar")
}

func TestKafkaSerializationWithLocalhostTraffic(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the feature is only supported on linux.")
	}
	var (
		clientPort = uint16(52800)
		serverPort = uint16(8080)
		localhost  = util.AddressFromString("127.0.0.1")
	)

	connections := []network.ConnectionStats{
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  clientPort,
			Dest:   localhost,
			DPort:  serverPort,
			Pid:    1,
		}},
		{ConnectionTuple: network.ConnectionTuple{
			Source: localhost,
			SPort:  serverPort,
			Dest:   localhost,
			DPort:  clientPort,
			Pid:    2,
		}},
	}

	const topicName = "TopicName"
	const apiVersion2 = 1
	kafkaKey := kafka.NewKey(
		localhost,
		localhost,
		clientPort,
		serverPort,
		topicName,
		kafka.FetchAPIKey,
		apiVersion2,
	)

	in := &network.Connections{
		BufferedData: network.BufferedData{
			Conns: connections,
		},
		Kafka: map[kafka.Key]*kafka.RequestStats{
			kafkaKey: {
				ErrorCodeToStat: map[int32]*kafka.RequestStat{0: {Count: 10, FirstLatencySample: 5}},
			},
		},
	}

	kafkaOut := &model.DataStreamsAggregations{
		KafkaAggregations: []*model.KafkaAggregation{
			{
				Header: &model.KafkaRequestHeader{
					RequestType:    kafka.FetchAPIKey,
					RequestVersion: apiVersion2,
				},
				Topic: topicName,
				StatsByErrorCode: map[int32]*model.KafkaStats{
					0: {Count: 10, FirstLatencySample: 5},
				},
			},
		},
	}

	kafkaOutBlob, err := proto.Marshal(kafkaOut)
	require.NoError(t, err)

	out := &model.Connections{
		Conns: []*model.Connection{
			{
				Laddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Raddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				DataStreamsAggregations: kafkaOutBlob,
				RouteIdx:                -1,
				Protocol:                marshal.FormatProtocolStack(protocols.Stack{}, 0),
				Pid:                     1,
			},
			{
				Laddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				DataStreamsAggregations: kafkaOutBlob,
				RouteIdx:                -1,
				Protocol:                marshal.FormatProtocolStack(protocols.Stack{}, 0),
				Pid:                     2,
			},
		},
		AgentConfiguration: &model.AgentConfiguration{
			NpmEnabled: false,
			UsmEnabled: false,
		},
	}

	blobWriter := getBlobWriter(t, assert.New(t), in, "application/protobuf")

	unmarshaler := unmarshal.GetUnmarshaler("application/protobuf")
	result, err := unmarshaler.Unmarshal(blobWriter.Bytes())
	require.NoError(t, err)

	require.Equal(t, out, result)
}
