// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"bytes"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"go.uber.org/atomic"
	"io"
	"math"
	"net/http/httptest"
	"testing"
	"time"

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
	ostream := bytes.NewBuffer(nil)

	connectionsModeler := encoding.NewConnectionsModeler(in)
	defer connectionsModeler.Close()

	err := marshaller.Marshal(in, ostream, connectionsModeler)
	require.NoError(t, err)

	writeConnections(rec, marshaller, in)

	rec.Flush()
	out := rec.Body.Bytes()
	assert.Equal(t, ostream.Bytes(), out)

}

func generateBenchMarkPayload(sourcePortsMax, destPortsMax uint16) *network.Connections {
	localhost := util.AddressFromString("127.0.0.1")

	payload := network.Connections{
		BufferedData: network.BufferedData{
			Conns: make([]network.ConnectionStats, uint32(sourcePortsMax)*uint32(destPortsMax)),
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
			index := uint32(sport)*uint32(sourcePortsMax) + uint32(dport)

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

	return &payload
}

func StreamConnections(runCounter *atomic.Uint64, reqID, contentType string, writer io.Writer, cs *network.Connections, batchSize int) error {
	start := time.Now()

	marshaler := encoding.GetMarshaler(contentType)
	connectionsModeler := encoding.NewConnectionsModeler(cs)
	connectionsModeler.SetBatchCount(int(math.Ceil(float64(len(cs.Conns)) / float64(batchSize))))
	logRequests(reqID, "/ws-connections", runCounter.Inc(), len(cs.Conns), start)

	// As long as there are connections, we divide them into batches and subsequently send all the batches
	// via a gRPC stream to the process agent. The size of each batch is determined by the value of batchSize.
	for len(cs.Conns) > 0 {
		finalBatchSize := min(batchSize, len(cs.Conns))
		rest := cs.Conns[finalBatchSize:]
		cs.Conns = cs.Conns[:finalBatchSize]

		if err := marshaler.Marshal(cs, writer, connectionsModeler); err != nil {
			return fmt.Errorf("unable to marshal payload due to: %s", err)
		}

		cs.Conns = rest
	}

	return nil
}

func commonBenchmarkHTTPEncoder(b *testing.B, numberOfPorts uint16, batchSize int) {
	runc := atomic.NewUint64(0)
	payload := generateBenchMarkPayload(numberOfPorts, numberOfPorts)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		StreamConnections(runc, "1", "application/protobuf", io.Discard, payload, batchSize)
	}
}

func BenchmarkHTTPEncoder10000Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 100, 600)
}

func BenchmarkHTTPEncoder10000RequestsSingleBatch(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 100, 100*100)
}

func BenchmarkHTTPEncoder1000000Requests(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 1000, 600)
}

func BenchmarkHTTPEncoder1000000RequestsSingleBatch(b *testing.B) {
	commonBenchmarkHTTPEncoder(b, 1000, 1000*1000)
}
