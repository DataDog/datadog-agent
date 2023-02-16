// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"

	"github.com/DataDog/datadog-agent/pkg/dogstatsd/packets"
)

func mockDemultiplexer() aggregator.Demultiplexer {
	return mockDemultiplexerWithFlushInterval(time.Millisecond * 10)
}

func mockDemultiplexerWithFlushInterval(d time.Duration) aggregator.Demultiplexer {
	opts := aggregator.DefaultAgentDemultiplexerOptions(nil)
	opts.FlushInterval = d
	opts.DontStartForwarders = true
	demux := aggregator.InitAndStartAgentDemultiplexer(opts, "hostname")
	return demux
}

func buildPacketContent(numberOfMetrics int, nbValuePerMessage int) []byte {
	values := ""
	for i := 0; i < nbValuePerMessage; i++ {
		values += ":666"
	}
	rawPacket := "daemon" + values + "|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2"
	packets := rawPacket
	for i := 1; i < numberOfMetrics; i++ {
		packets += "\n" + rawPacket
	}
	return []byte(packets)
}

func benchParsePackets(b *testing.B, rawPacket []byte) {
	runWithComponent(b, func(c Component) {
		s := c.(*server)
		// our logger will log dogstatsd packet by default if nothing is setup
		config.SetupLogger("", "off", "", "", false, true, false)

		demux := aggregator.InitTestAgentDemultiplexer()
		defer demux.Stop(false)
		_ = s.Start(demux)
		defer s.Stop()

		done := make(chan struct{})
		go func() {
			s, l := demux.WaitForSamples(time.Millisecond * 1)
			if len(s) > 0 || len(l) > 0 {
				return
			}
		}()
		defer close(done)

		b.RunParallel(func(pb *testing.PB) {
			batcher := newBatcher(demux.AgentDemultiplexer)
			parser := newParser(newFloat64ListPool())
			packet := packets.Packet{
				Contents: rawPacket,
				Origin:   packets.NoOrigin,
			}

			packets := packets.Packets{&packet}
			samples := make([]metrics.MetricSample, 0, 512)
			for pb.Next() {
				packet.Contents = rawPacket
				samples = s.parsePackets(batcher, parser, packets, samples)
			}
		})
	})
}

func BenchmarkParsePackets(b *testing.B) {
	// 640 packets of 1 samples
	benchParsePackets(b, buildPacketContent(20*32, 1))
}

func BenchmarkParsePacketsMultiple(b *testing.B) {
	// 64 packets of 10 samples
	benchParsePackets(b, buildPacketContent(2*32, 10))
}

var samplesBench []metrics.MetricSample

func BenchmarkPbarseMetricMessage(b *testing.B) {
	runWithComponent(b, func(c Component) {
		s := c.(*server)
		// our logger will log dogstatsd packet by default if nothing is setup
		config.SetupLogger("", "off", "", "", false, true, false)

		demux := aggregator.InitTestAgentDemultiplexer()
		_ = s.Start(demux)
		defer s.Stop()

		done := make(chan struct{})
		go func() {
			s, l := demux.WaitForSamples(time.Millisecond * 1)
			if len(s) > 0 || len(l) > 0 {
				return
			}
		}()
		defer close(done)

		parser := newParser(newFloat64ListPool())
		message := []byte("daemon:666|h|@0.5|#sometag1:somevalue1,sometag2:somevalue2")

		b.RunParallel(func(pb *testing.PB) {
			samplesBench = make([]metrics.MetricSample, 0, 512)
			for pb.Next() {
				s.parseMetricMessage(samplesBench, parser, message, "", false)
				samplesBench = samplesBench[0:0]
			}
		})
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
	runWithComponent(b, func(c Component) {
		s := c.(*server)
		port, err := getAvailableUDPPort()
		require.NoError(b, err)
		config.Datadog.SetDefault("dogstatsd_port", port)

		// our logger will log dogstatsd packet by default if nothing is setup
		config.SetupLogger("", "off", "", "", false, true, false)

		demux := aggregator.InitTestAgentDemultiplexer()
		_ = s.Start(demux)
		defer s.Stop()

		done := make(chan struct{})
		go func() {
			s, l := demux.WaitForSamples(time.Millisecond * 1)
			if len(s) > 0 || len(l) > 0 {
				return
			}
		}()
		defer close(done)

		batcher := newBatcher(demux.AgentDemultiplexer)
		parser := newParser(newFloat64ListPool())

		samples := make([]metrics.MetricSample, 0, 512)
		for n := 0; n < b.N; n++ {
			packet := packets.Packet{
				Contents: []byte("airflow.job.duration.my_job_type.my_job_name:666|g"),
				Origin:   packets.NoOrigin,
			}
			packets := packets.Packets{&packet}
			samples = s.parsePackets(batcher, parser, packets, samples)
		}

		b.ReportAllocs()
	})
}
