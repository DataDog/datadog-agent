// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
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
	batchSize := config.Datadog.GetInt("dogstatsd_no_aggregation_pipeline_batch_size")

	opts := demuxTestOptions()
	demux := initAgentDemultiplexer(opts, "")

	batch := testDemuxSamples(t)

	demux.AddLateMetrics(batch)

	for i := 0; i < batchSize; i++ {
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Name)
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Host)
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Tags)
		require.Zero(demux.statsd.noAggWorker.currentBatch[i].Value)
	}
	require.Equal(demux.statsd.noAggWorker.currentBatchIdx, 0)
	require.Len(demux.statsd.workers[0].samplesChan, 1)
	read := <-demux.statsd.workers[0].samplesChan
	require.Len(read, 3)
}

// the option is enabled, these metrics will go through the no aggregation pipeline.
func TestDemuxNoAggOptionEnabled(t *testing.T) {
	require := require.New(t)
	batchSize := config.Datadog.GetInt("dogstatsd_no_aggregation_pipeline_batch_size")

	opts := demuxTestOptions()
	opts.EnableNoAggregationPipeline = true
	demux := initAgentDemultiplexer(opts, "")

	batch := testDemuxSamples(t)

	demux.AddLateMetrics(batch)

	require.Len(demux.statsd.noAggWorker.currentBatch, batchSize)
	require.Equal(demux.statsd.noAggWorker.currentBatchIdx, 3)
	require.Equal(demux.statsd.noAggWorker.currentBatch[0], batch[0])
	require.Equal(demux.statsd.noAggWorker.currentBatch[1], batch[1])
	require.Equal(demux.statsd.noAggWorker.currentBatch[2], batch[2])
	for i := 3; i < batchSize; i++ {
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Name)
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Host)
		require.Empty(demux.statsd.noAggWorker.currentBatch[i].Tags)
		require.Zero(demux.statsd.noAggWorker.currentBatch[i].Value)
	}
	require.Len(demux.statsd.workers[0].samplesChan, 0)
}

func TestDemuxNoAggOptionIsDisabledByDefault(t *testing.T) {
	opts := demuxTestOptions()
	demux := InitAndStartAgentDemultiplexer(opts, "")
	require.False(t, demux.Options().EnableNoAggregationPipeline, "the no aggregation pipeline should be disabled by default")
	demux.Stop(false)
}
