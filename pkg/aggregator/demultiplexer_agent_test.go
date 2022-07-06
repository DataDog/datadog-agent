// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/stretchr/testify/require"
)

func testDemuxSamples(t *testing.T) metrics.MetricSampleBatch {
	batch := metrics.MetricSampleBatch{
		metrics.MetricSample{
			Name:      "first",
			Value:     1,
			Mtype:     metrics.GaugeType,
			Timestamp: 1657099120.0,
		},
		metrics.MetricSample{
			Name:      "second",
			Value:     20,
			Mtype:     metrics.CountType,
			Timestamp: 1657099125.0,
		},
		metrics.MetricSample{
			Name:      "third",
			Value:     60,
			Mtype:     metrics.CountType,
			Timestamp: 1657099125.0,
		},
	}
	return batch
}

// the option is NOT enabled, this metric should go into the first
// timesampler of the statsd stack.
func TestDemuxNoAggOptionDisabled(t *testing.T) {
	require := require.New(t)

	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")
	defer demux.Stop(false)

	batch := testDemuxSamples(t)

	demux.AddLateMetrics(batch)

	require.Len(demux.statsd.lateMetrics, 0)
	require.Len(demux.statsd.workers[0].samplesChan, 1)
	read := <-demux.statsd.workers[0].samplesChan
	require.Len(read, 3)
}

// the option is enabled, these metrics will go through the late samples pipeline.
func TestDemuxNoAggOptionEnabled(t *testing.T) {
	require := require.New(t)

	opts := demuxTestOptions()
	opts.EnableNoAggregationPipeline = true
	demux := InitAndStartAgentDemultiplexer(opts, "")
	defer demux.Stop(false)

	batch := testDemuxSamples(t)

	demux.AddLateMetrics(batch)

	require.Len(demux.statsd.lateMetrics, 3)
	require.Len(demux.statsd.workers[0].samplesChan, 0)
}

func TestDemuxNoAggOptionIsDisabledByDefault(t *testing.T) {
	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")
	require.False(t, demux.Options().EnableNoAggregationPipeline, "the no aggregation pipeline should be disabled by default")
	demux.Stop(false)
}
