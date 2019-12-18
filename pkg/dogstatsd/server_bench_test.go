// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package dogstatsd

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/listeners"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func mockAggregator(pool *metrics.MetricSamplePool) (chan []metrics.MetricSample, chan []*metrics.Event, chan []*metrics.ServiceCheck) {
	bufferedMetricIn := make(chan []metrics.MetricSample, 100)
	bufferedServiceCheckIn := make(chan []*metrics.ServiceCheck, 100)
	bufferedEventIn := make(chan []*metrics.Event, 100)

	go func() {
		for {
			select {
			case _ = <-bufferedServiceCheckIn:
				break
			case _ = <-bufferedEventIn:
				break
			case sampleBatch := <-bufferedMetricIn:
				pool.PutBatch(sampleBatch)
			}
		}
	}()

	return bufferedMetricIn, bufferedEventIn, bufferedServiceCheckIn
}

func buildPacketConent(numberOfMetrics int) []byte {
	rawPacket := "daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"
	for i := 1; i < numberOfMetrics; i++ {
		rawPacket += "\ndaemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"
	}
	return []byte(rawPacket)
}

func BenchmarkParsePackets(b *testing.B) {
	// our logger will log dogstatsd packet by default if nothing is setup
	config.SetupLogger("", "off", "", "", false, true, false)

	pool := metrics.NewMetricSamplePool(16)
	sampleOut, eventOut, scOut := mockAggregator(pool)
	s, _ := NewServer(pool, sampleOut, eventOut, scOut)
	defer s.Stop()

	b.RunParallel(func(pb *testing.PB) {
		batcher := newBatcher(pool, sampleOut, eventOut, scOut)
		// 32 packets of 20 samples
		rawPacket := buildPacketConent(20 * 32)
		packet := listeners.Packet{
			Contents: rawPacket,
			Origin:   listeners.NoOrigin,
		}
		packets := listeners.Packets{&packet}
		for pb.Next() {
			packet.Contents = rawPacket
			s.parsePackets(batcher, packets)
		}
	})
}
