// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && linux_bpf

package encoding

import (
	"fmt"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/marshal"
	"github.com/DataDog/datadog-agent/pkg/network/encoding/unmarshal"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/kafka"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

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
		USMData: network.USMProtocolsData{
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
				ResolvConfIdx:     -1,
				Protocol:          marshal.FormatProtocolStack(protocols.Stack{}, 0),
			},
			{
				Laddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:             &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				Http2Aggregations: http2OutBlob,
				RouteIdx:          -1,
				ResolvConfIdx:     -1,
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
			in.USMData.HTTP2 = map[http.Key]*http.RequestStats{httpKey: {}}
			out := encodeAndDecodeHTTP2(in)

			require.NotNil(t, out)
			require.Len(t, out.EndpointAggregations, 1)
			require.Equal(t, httpKey.Path.Content.Get(), out.EndpointAggregations[0].Path)
		} else {
			// No HTTP2 data in this payload, so we should never get HTTP2 data back after the serialization
			in.USMData.HTTP2 = nil
			out := encodeAndDecodeHTTP2(in)
			require.Nil(t, out, "expected a nil object, but got garbage")
		}
	}
}

func TestKafkaSerializationWithLocalhostTraffic(t *testing.T) {
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
		USMData: network.USMProtocolsData{
			Kafka: map[kafka.Key]*kafka.RequestStats{
				kafkaKey: {
					ErrorCodeToStat: map[int32]*kafka.RequestStat{0: {Count: 10, FirstLatencySample: 5}},
				},
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
				ResolvConfIdx:           -1,
				Protocol:                marshal.FormatProtocolStack(protocols.Stack{}, 0),
				Pid:                     1,
			},
			{
				Laddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(serverPort)},
				Raddr:                   &model.Addr{Ip: "127.0.0.1", Port: int32(clientPort)},
				DataStreamsAggregations: kafkaOutBlob,
				RouteIdx:                -1,
				ResolvConfIdx:           -1,
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
