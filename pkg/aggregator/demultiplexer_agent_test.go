// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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
	deps := createDemultiplexerAgentTestDeps(t)

	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.Compressor, deps.Tagger, "")

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
	mockSerializer.On("AreSeriesEnabled").Return(true)
	mockSerializer.On("AreSketchesEnabled").Return(true)
	opts.EnableNoAggregationPipeline = true
	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.Compressor, deps.Tagger, "")
	demux.statsd.noAggStreamWorker.serializer = mockSerializer // the no agg pipeline will use our mocked serializer

	go demux.run()

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
	deps := fxutil.Test[TestDeps](t, defaultforwarder.MockModule(), core.MockBundle(), compressionimpl.MockModule())
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

type DemultiplexerAgentTestDeps struct {
	TestDeps
	OrchestratorFwd orchestratorforwarder.Component
	EventPlatform   eventplatform.Component
	Compressor      compression.Component
	Tagger          tagger.Component
}

func createDemultiplexerAgentTestDeps(t *testing.T) DemultiplexerAgentTestDeps {
	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())

	return fxutil.Test[DemultiplexerAgentTestDeps](
		t,
		defaultforwarder.MockModule(),
		core.MockBundle(),
		orchestratorimpl.MockModule(),
		eventplatformimpl.MockModule(),
		compressionimpl.MockModule(),
		fx.Provide(func() tagger.Component { return taggerComponent }),
	)
}
