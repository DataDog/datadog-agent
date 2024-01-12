// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/log/logimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

//nolint:revive // TODO(AML) Fix revive linter
func testDemuxSamples(t *testing.T) metrics.MetricSampleBatch {
	batch := metrics.MetricSampleBatch{
		metrics.MetricSample{
			Name:      "first",
			Value:     1,
			Mtype:     metrics.GaugeType,
			Timestamp: 1657099120.0,
			Tags:      []string{"tag:1", "tag:2"},
		},
		metrics.MetricSample{
			Name:      "second",
			Value:     20,
			Mtype:     metrics.CounterType,
			Timestamp: 1657099125.0,
			Tags:      []string{"tag:3", "tag:4"},
		},
		metrics.MetricSample{
			Name:      "third",
			Value:     60,
			Mtype:     metrics.CounterType,
			Timestamp: 1657099125.0,
			Tags:      []string{"tag:5"},
		},
	}
	return batch
}

// the option is NOT enabled, this metric should go into the first
// timesampler of the statsd stack.
func TestDemuxNoAggOptionDisabled(t *testing.T) {
	require := require.New(t)

	opts := demuxTestOptions()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	demux := initAgentDemultiplexer(log, NewForwarderTest(log), &orchestratorForwarder, opts, "")

	batch := testDemuxSamples(t)

	demux.SendSamplesWithoutAggregation(batch)
	require.Len(demux.statsd.workers[0].samplesChan, 1)
	read := <-demux.statsd.workers[0].samplesChan
	require.Len(read, 3)
}

// the option is enabled, these metrics will go through the no aggregation pipeline.
func TestDemuxNoAggOptionEnabled(t *testing.T) {
	require := require.New(t)

	noAggWorkerStreamCheckFrequency = 100 * time.Millisecond

	opts := demuxTestOptions()
	mockSerializer := &MockSerializerIterableSerie{}
	opts.EnableNoAggregationPipeline = true
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	demux := initAgentDemultiplexer(log, NewForwarderTest(log), &orchestratorForwarder, opts, "")
	demux.statsd.noAggStreamWorker.serializer = mockSerializer // the no agg pipeline will use our mocked serializer

	go demux.Run()

	batch := testDemuxSamples(t)

	demux.SendSamplesWithoutAggregation(batch)
	time.Sleep(200 * time.Millisecond) // give some time for the automatic flush to trigger
	demux.Stop(true)

	// nothing should be in the time sampler
	require.Len(demux.statsd.workers[0].samplesChan, 0)
	require.Len(mockSerializer.series, 3)

	for i := 0; i < len(batch); i++ {
		require.Equal(batch[i].Name, mockSerializer.series[i].Name)
		require.Len(mockSerializer.series[i].Points, 1)
		require.Equal(batch[i].Timestamp, mockSerializer.series[i].Points[0].Ts)
		require.ElementsMatch(batch[i].Tags, mockSerializer.series[i].Tags.UnsafeToReadOnlySliceString())
	}
}

func TestDemuxNoAggOptionIsDisabledByDefault(t *testing.T) {
	opts := demuxTestOptions()
	deps := fxutil.Test[TestDeps](t, defaultforwarder.MockModule(), config.MockModule(), logimpl.MockModule())
	demux := InitAndStartAgentDemultiplexerForTest(deps, opts, "")

	require.False(t, demux.Options().EnableNoAggregationPipeline, "the no aggregation pipeline should be disabled by default")
	demux.Stop(false)
}

func TestMetricSampleTypeConversion(t *testing.T) {
	require := require.New(t)

	tests := []struct {
		metricType    metrics.MetricType
		apiMetricType metrics.APIMetricType
		supported     bool
	}{
		{metrics.GaugeType, metrics.APIGaugeType, true},
		{metrics.CounterType, metrics.APIRateType, true},
		{metrics.RateType, metrics.APIRateType, true},
		{metrics.MonotonicCountType, metrics.APIGaugeType, false},
		{metrics.CountType, metrics.APIGaugeType, false},
		{metrics.HistogramType, metrics.APIGaugeType, false},
		{metrics.HistorateType, metrics.APIGaugeType, false},
		{metrics.SetType, metrics.APIGaugeType, false},
		{metrics.DistributionType, metrics.APIGaugeType, false},
	}

	for _, test := range tests {
		ms := metrics.MetricSample{Mtype: test.metricType}
		rv, supported := metricSampleAPIType(ms)

		if test.supported {
			require.True(supported, fmt.Sprintf("Metric type %s should be supported", test.metricType.String()))
		} else {
			require.False(supported, fmt.Sprintf("Metric type %s should be not supported", test.metricType.String()))
		}
		require.Equal(test.apiMetricType, rv, fmt.Sprintf("Wrong conversion for %s", test.metricType.String()))
	}
}

func TestStatusOutPut(t *testing.T) {
	require := require.New(t)

	tests := []struct {
		name       string
		assertFunc func(a *AgentDemultiplexer)
	}{
		{"JSON", func(a *AgentDemultiplexer) {
			stats := make(map[string]interface{})
			a.JSON(stats)

			require.NotEmpty(stats)
		}},
		{"Text", func(a *AgentDemultiplexer) {
			b := new(bytes.Buffer)
			err := a.Text(b)

			require.NoError(err)

			require.NotEmpty(b.String())
		}},
		{"HTML", func(a *AgentDemultiplexer) {
			b := new(bytes.Buffer)
			err := a.HTML(b)

			require.NoError(err)

			require.NotEmpty(b.String())
		}},
	}

	opts := demuxTestOptions()
	log := fxutil.Test[log.Component](t, logimpl.MockModule())
	orchestratorForwarder := optional.NewOption[defaultforwarder.Forwarder](defaultforwarder.NoopForwarder{})
	demux := initAgentDemultiplexer(log, NewForwarderTest(log), &orchestratorForwarder, opts, "")

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.assertFunc(demux)
		})
	}
}
