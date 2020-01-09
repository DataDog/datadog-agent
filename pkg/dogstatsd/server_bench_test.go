// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package dogstatsd

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

func BenchmarkWithMapper(b *testing.B) {
	datadogYaml := `
dogstatsd_mapper_profiles:
  - name: airflow
    prefix: 'airflow.'
    mappings:
      - match: "airflow.job.duration.*.*"       # metric format: airflow.job.duration.<job_type>.<job_name>
        name: "airflow.job.duration"            # remap the metric name
        tags:
          job_type: "$1"
          job_name: "$2"
      - match: "airflow.job.size.*.*"           # metric format: airflow.job.size.<job_type>.<job_name>
        name: "airflow.job.size"                # remap the metric name
        tags:
          foo: "$1"
          bar: "$2"
`
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.ReadConfig(strings.NewReader(datadogYaml))
	assert.NoError(b, err)

	BenchmarkMapperControl(b)
}

func BenchmarkMapperControl(b *testing.B) {
	port, err := getAvailableUDPPort()
	require.NoError(b, err)
	config.Datadog.SetDefault("dogstatsd_port", port)

	// our logger will log dogstatsd packet by default if nothing is setup
	config.SetupLogger("", "off", "", "", false, true, false)

	pool := metrics.NewMetricSamplePool(16)
	sampleOut, eventOut, scOut := mockAggregator(pool)
	s, _ := NewServer(pool, sampleOut, eventOut, scOut)
	defer s.Stop()

	batcher := newBatcher(pool, sampleOut, eventOut, scOut)

	for n := 0; n < b.N; n++ {
		packet := listeners.Packet{
			Contents: []byte("airflow.job.duration.my_job_type.my_job_name:666|g"),
			Origin:   listeners.NoOrigin,
		}
		packets := listeners.Packets{&packet}
		s.parsePackets(batcher, packets)
	}

	b.ReportAllocs()
}
