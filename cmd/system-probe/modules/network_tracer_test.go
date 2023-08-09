// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/encoding"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestDecode(t *testing.T) {
	rec := httptest.NewRecorder()

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
						SentBytes:   2,
						RecvBytes:   101,
						Retransmits: 201,
					},
					LastUpdateEpoch: 50,
					Pid:             6000,
					NetNS:           7,
					SPort:           1000,
					DPort:           9000,
					IPTranslation: &network.IPTranslation{
						ReplSrcIP:   util.AddressFromString("20.1.1.1"),
						ReplDstIP:   util.AddressFromString("20.1.1.1"),
						ReplSrcPort: 40,
						ReplDstPort: 70,
					},

					Type:      network.UDP,
					Family:    network.AFINET6,
					Direction: network.LOCAL,
				},
			},
		},
	}

	marshaller := encoding.GetMarshaler(encoding.ContentTypeJSON)
	modeler := marshaller.InitModeler(in)
	payload := marshaller.Model(in, modeler)
	expected, err := marshaller.Marshal(payload)
	require.NoError(t, err)

	writeConnections(rec, marshaller, in)

	rec.Flush()
	out := rec.Body.Bytes()
	assert.Equal(t, expected, out)

}

//type connTag = uint64

//// ConnTag constant must be the same for all platform
//const (
//	tagGnuTLS  connTag = 0x01 // network.ConnTagGnuTLS
//	tagOpenSSL connTag = 0x02 // network.ConnTagOpenSSL
//	tagTLS     connTag = 0x10 // network.ConnTagTLS
//)

//func TestDecode2(t *testing.T) {
//	rec := httptest.NewRecorder()
//
//	//cs := &network.Connections{
//	//	BufferedData: network.BufferedData{
//	//		Conns: []network.ConnectionStats{
//	//			{
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			},
//	//			{
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			},
//	//			{
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			}, {
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			},
//	//			{
//	//				Source: util.AddressFromString("10.1.1.1"),
//	//				Dest:   util.AddressFromString("10.2.2.2"),
//	//				Monotonic: network.StatCounters{
//	//					SentBytes:   1,
//	//					RecvBytes:   100,
//	//					Retransmits: 201,
//	//				},
//	//				Last: network.StatCounters{
//	//					SentBytes:   2,
//	//					RecvBytes:   101,
//	//					Retransmits: 201,
//	//				},
//	//				LastUpdateEpoch: 50,
//	//				Pid:             6000,
//	//				NetNS:           7,
//	//				SPort:           1000,
//	//				DPort:           9000,
//	//				IPTranslation: &network.IPTranslation{
//	//					ReplSrcIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplDstIP:   util.AddressFromString("20.1.1.1"),
//	//					ReplSrcPort: 40,
//	//					ReplDstPort: 70,
//	//				},
//	//
//	//				Type:      network.UDP,
//	//				Family:    network.AFINET6,
//	//				Direction: network.LOCAL,
//	//			},
//	//		},
//	//	},
//	//}
//
//	httpReqStats := http.NewRequestStats(true)
//
//	httpReqStats.AddRequest(100, 12.5, 0, nil)
//	httpReqStats.AddRequest(100, 12.5, tagGnuTLS, nil)
//	httpReqStats.AddRequest(405, 3.5, tagOpenSSL, nil)
//	httpReqStats.AddRequest(405, 3.5, 0, nil)
//
//	// Verify the latency data is correct prior to serialization
//
//	latencies := httpReqStats.Data[httpReqStats.NormalizeStatusCode(100)].Latencies
//	assert.Equal(t, 2.0, latencies.GetCount())
//	//verifyQuantile(t, latencies, 0.5, 12.5)
//
//	latencies = httpReqStats.Data[httpReqStats.NormalizeStatusCode(405)].Latencies
//	assert.Equal(t, 2.0, latencies.GetCount())
//	//verifyQuantile(t, latencies, 0.5, 3.5)
//
//	key := http.NewKey(
//		util.AddressFromString("10.1.1.1"),
//		util.AddressFromString("10.2.2.2"),
//		60000,
//		80,
//		"/testpath",
//		true,
//		http.MethodGet,
//	)
//
//	key2 := http.NewKey(
//		util.AddressFromString("10.1.1.3"),
//		util.AddressFromString("10.2.2.4"),
//		60002,
//		80,
//		"/testpath2",
//		true,
//		http.MethodGet,
//	)
//
//	key3 := http.NewKey(
//		util.AddressFromString("10.1.1.5"),
//		util.AddressFromString("10.2.2.6"),
//		60003,
//		80,
//		"/testpath3",
//		true,
//		http.MethodGet,
//	)
//
//	payload := &network.Connections{
//		BufferedData: network.BufferedData{
//			Conns: []network.ConnectionStats{
//				{
//					Source: util.AddressFromString("10.1.1.1"),
//					Dest:   util.AddressFromString("10.2.2.2"),
//					SPort:  60000,
//					DPort:  80,
//				},
//				{
//					Source: util.AddressFromString("10.1.1.3"),
//					Dest:   util.AddressFromString("10.2.2.4"),
//					SPort:  60002,
//					DPort:  80,
//				},
//				{
//					Source: util.AddressFromString("10.1.1.5"),
//					Dest:   util.AddressFromString("10.2.2.6"),
//					SPort:  60003,
//					DPort:  80,
//				},
//			},
//		},
//		HTTP: map[http.Key]*http.RequestStats{
//			key:  httpReqStats,
//			key2: httpReqStats,
//			key3: httpReqStats,
//		},
//	}
//	//for len(payload.Conns) > 0 {
//	//	maxConnsPerMessage := 2
//	//	finalBatchSize := min(maxConnsPerMessage, len(payload.Conns))
//	//	rest := payload.Conns[finalBatchSize:]
//	//	payload.Conns = payload.Conns[:finalBatchSize]
//	//	marshaler := encoding.GetMarshaler(encoding.ContentTypeProtobuf)
//	//	conns, err := getConnectionsFromMarshler(marshaler, payload)
//	//	require.NoError(t, err)
//	//	assert.Equal(t, len(conns), 202)
//	//
//	//	println(conns)
//	//	payload.Conns = rest
//	//}
//
//	//marshaller := encoding.GetMarshaler(encoding.ContentTypeJSON)
//	//expected, err := marshaller.Marshal(payload)
//	//require.NoError(t, err)
//	//
//	//writeConnections(rec, marshaller, payload)
//	//
//	//rec.Flush()
//	//out := rec.Body.Bytes()
//	//assert.Equal(t, expected, out)
//
//}
