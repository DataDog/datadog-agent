// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/orchestratorimpl"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
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

	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

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
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
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
	deps := fxutil.Test[TestDeps](t,
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwarder.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		haagentmock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistmock.MockModule(),
	)
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
		require.Equal(test.apiMetricType, rv, "Wrong conversion for "+test.metricType.String())
	}
}

func TestUpdateTagFilterList(t *testing.T) {
	require := require.New(t)

	mockConfig := configmock.New(t)
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	filterList := filterlistimpl.NewFilterList(deps.Log, mockConfig, deps.Telemetry)
	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "exclude",
			Tags:   []string{"tag1", "tag2"},
		}})

	demux := InitAndStartAgentDemultiplexer(
		deps.Log,
		NewForwarderTest(deps.Log),
		deps.OrchestratorFwd,
		opts,
		deps.EventPlatform,
		deps.HaAgent,
		deps.Compressor,
		deps.Tagger,
		filterList,
		"",
	)

	// Set up a mock serializer so we con examine the metrics sent to it.
	s := &MockSerializerSketch{}
	s.On("AreSeriesEnabled").Return(true)
	s.On("AreSketchesEnabled").Return(true)
	s.On("SendServiceChecks", mock.Anything).Return(nil)

	demux.aggregator.serializer = s
	demux.sharedSerializer = s

	testCountBlocked := func(expected []string, ts float64) {
		demux.AggregateSample(metrics.MetricSample{
			Name:      "dist.metric",
			Value:     42,
			Mtype:     metrics.DistributionType,
			Timestamp: ts,
			Tags:      []string{"tag1:one", "tag2:two", "tag3:three", "tag4:four"},
		})

		require.Eventually(func() bool {
			return len(demux.statsd.workers[0].samplesChan) == 0
		}, time.Second, time.Millisecond)
		demux.ForceFlushToSerializer(time.Unix(int64(ts+30), 0), true)

		metric := slices.IndexFunc(s.sketches, func(serie *metrics.SketchSeries) bool {
			return serie.Name == "dist.metric"
		})

		require.NotEqualf(-1, metric, "dist.metric not found in %+v", s.sketches)
		tags := strings.Split(s.sketches[metric].Tags.Join(","), ",")
		require.ElementsMatch(expected, tags)
	}

	// After initial setup, we have filterlist from the configuration file.
	// It may take a little time as it has to be sent to a separate routine.
	require.Eventually(func() bool {
		return len(demux.aggregator.tagfilterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the tagfilterList update")

	// Tag 1 and 2 are excluded
	testCountBlocked([]string{"tag3:three", "tag4:four"}, 32.0)

	// Reset the mock
	s.sketches = []*metrics.SketchSeries{}

	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "exclude",
			Tags:   []string{"tag4", "tag5"},
		}})

	// Ensure the new filter list has been sent.
	require.Eventually(func() bool {
		return len(demux.aggregator.tagfilterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the tagfilterList update")

	testCountBlocked([]string{"tag1:one", "tag2:two", "tag3:three"}, 62.0)

	demux.Stop(false)

	// We no longer need to ensure the correct metrics are being blocked after stopping. Just make sure it doesn't deadlock.
	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "include",
			Tags:   []string{"thing"},
		}})

	// Wait until the aggregator has been removed whilst stopping demux.
	require.Eventually(func() bool {
		return demux.aggregator == nil
	}, time.Second, time.Millisecond)

	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "exclude",
			Tags:   []string{"thang"},
		}})

}

func TestUpdateMetricFilterList(t *testing.T) {
	require := require.New(t)

	mockConfig := configmock.New(t)
	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)
	filterList := filterlistimpl.NewFilterList(deps.Log, mockConfig, deps.Telemetry)
	filterList.SetMetricFilterList([]string{"original.blocked.count"}, false)

	demux := InitAndStartAgentDemultiplexer(
		deps.Log,
		NewForwarderTest(deps.Log),
		deps.OrchestratorFwd,
		opts,
		deps.EventPlatform,
		deps.HaAgent,
		deps.Compressor,
		deps.Tagger,
		filterList,
		"",
	)

	// Set up a mock serializer so we con examine the metrics sent to it.
	s := &MockSerializerIterableSerie{}
	s.On("AreSeriesEnabled").Return(true)
	s.On("AreSketchesEnabled").Return(true)
	s.On("SendServiceChecks", mock.Anything).Return(nil)

	demux.aggregator.serializer = s
	demux.sharedSerializer = s

	testCountBlocked := func(blockCount bool, ts float64) {
		// Send a histogram, flush it and test the output
		// If blockedCount is true we test count is blocked and not avg.
		// If blockedCount is false we test avg is blocked and not count.
		demux.AggregateSample(metrics.MetricSample{
			Name: "original.blocked", Value: 42, Mtype: metrics.HistogramType, Timestamp: ts,
		})

		require.Eventually(func() bool {
			return len(demux.statsd.workers[0].samplesChan) == 0
		}, time.Second, time.Millisecond)
		demux.ForceFlushToSerializer(time.Unix(int64(ts+30), 0), true)

		// We should always contain the average of the histogram.
		require.Equal(blockCount, slices.ContainsFunc(s.series, func(serie *metrics.Serie) bool {
			return serie.Name == "original.blocked.avg"
		}))

		// Test if the count is filtered out.
		require.Equal(!blockCount, slices.ContainsFunc(s.series, func(serie *metrics.Serie) bool {
			return serie.Name == "original.blocked.count"
		}))
	}

	// After initial setup, we have filterlist from the configuration file.
	// It may take a little time as it has to be sent to a separate routine.
	require.Eventually(func() bool {
		return len(demux.aggregator.filterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the filterlist update")

	testCountBlocked(true, 32.0)

	// Reset the mock
	s.series = []*metrics.Serie{}

	filterList.SetMetricFilterList([]string{"original.blocked.avg"}, false)

	// Ensure the new filter list has been sent.
	require.Eventually(func() bool {
		return len(demux.aggregator.filterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the filterlist update")

	testCountBlocked(false, 62.0)

	demux.Stop(false)

	// We no longer need to ensure the correct metrics are being blocked after stopping. Just make sure it doesn't deadlock.
	filterList.SetMetricFilterList([]string{"another.metric"}, false)

	// Wait until the aggregator has been removed whilst stopping demux.
	require.Eventually(func() bool {
		return demux.aggregator == nil
	}, time.Second, time.Millisecond)

	filterList.SetMetricFilterList([]string{"more.metric"}, false)
}

type DemultiplexerAgentTestDeps struct {
	TestDeps
	OrchestratorFwd orchestratorforwarder.Component
	EventPlatform   eventplatform.Component
	Compressor      compression.Component
	Tagger          tagger.Component
	HaAgent         haagent.Component
	Telemetry       telemetry.Component
}

func createDemultiplexerAgentTestDeps(t *testing.T) DemultiplexerAgentTestDeps {
	taggerComponent := taggerfxmock.SetupFakeTagger(t)

	return fxutil.Test[DemultiplexerAgentTestDeps](
		t,
		fx.Provide(func() secrets.Component { return secretsmock.New(t) }),
		defaultforwarder.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		orchestratorimpl.MockModule(),
		eventplatformimpl.MockModule(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		haagentmock.Module(),
		filterlistmock.MockModule(),
		fx.Provide(func() tagger.Component { return taggerComponent }),
	)
}
