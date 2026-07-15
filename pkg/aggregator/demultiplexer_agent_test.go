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
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
	filterlistimpl "github.com/DataDog/datadog-agent/comp/filterlist/impl"
	defaultforwardermock "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/mock"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformmock "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/mock"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/def"
	orchestratormock "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator/mock"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	haagentmock "github.com/DataDog/datadog-agent/comp/haagent/mock"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/fx-mock"
	compression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/fx-mock"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func testDemuxSamples(_ *testing.T) metrics.MetricSampleBatch {
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
	opts.NoAggregationPipelineWorkersCount = 1
	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	demux.statsd.noAggStreamWorkers[0].serializer = mockSerializer // the no agg pipeline will use our mocked serializer

	go demux.run()

	batch := testDemuxSamples(t)

	demux.SendSamplesWithoutAggregation(batch)
	time.Sleep(200 * time.Millisecond) // give some time for the automatic flush to trigger
	demux.Stop()

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
		defaultforwardermock.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		haagentmock.Module(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		filterlistmock.MockModule(),
	)
	demux := InitAndStartAgentDemultiplexerForTest(deps, opts, "")

	require.Equal(t, 0, demux.Options().NoAggregationPipelineWorkersCount, "the no aggregation pipeline should be disabled by default")
	demux.Stop()
}

func TestDemuxNoAggWorkersCount(t *testing.T) {
	tests := []struct {
		name          string
		configured    int
		expectedCount int
	}{
		{
			name:          "configured count",
			configured:    3,
			expectedCount: 3,
		},
		{
			name:          "zero disables no aggregation workers",
			configured:    0,
			expectedCount: 0,
		},
		{
			name:          "negative disables no aggregation workers",
			configured:    -2,
			expectedCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := demuxTestOptions()
			opts.NoAggregationPipelineWorkersCount = tt.configured
			deps := createDemultiplexerAgentTestDeps(t)

			demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

			require.Len(t, demux.statsd.noAggStreamWorkers, tt.expectedCount)
			require.Len(t, demux.noAggSerializers, tt.expectedCount)
			if tt.expectedCount == 0 {
				require.Nil(t, demux.statsd.noAggSamplesChan)
				return
			}
			require.NotNil(t, demux.statsd.noAggSamplesChan)
			for _, worker := range demux.statsd.noAggStreamWorkers {
				require.Equal(t, demux.statsd.noAggSamplesChan, worker.samplesChan)
			}
		})
	}
}

func TestDemuxNoAggWorkersUseSharedQueue(t *testing.T) {
	opts := demuxTestOptions()
	opts.NoAggregationPipelineWorkersCount = 3
	deps := createDemultiplexerAgentTestDeps(t)

	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	for i := 0; i < 5; i++ {
		demux.SendSamplesWithoutAggregation(metrics.MetricSampleBatch{
			{
				Name:      fmt.Sprintf("metric.%d", i),
				Value:     float64(i),
				Mtype:     metrics.GaugeType,
				Timestamp: 1657099120.0,
			},
		})
	}

	require.Len(t, demux.statsd.noAggSamplesChan, 5)

	for i := 0; i < 5; i++ {
		batch := <-demux.statsd.noAggSamplesChan
		require.Len(t, batch, 1)
		require.Equal(t, fmt.Sprintf("metric.%d", i), batch[0].Name)
	}
}

func TestSendSamplesWithoutAggregationDropsEmptyBatch(t *testing.T) {
	opts := demuxTestOptions()
	opts.NoAggregationPipelineWorkersCount = 1
	deps := createDemultiplexerAgentTestDeps(t)

	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")

	demux.SendSamplesWithoutAggregation(metrics.MetricSampleBatch{})

	require.Len(t, demux.statsd.noAggSamplesChan, 0)
}

func TestAddAgentStartupTelemetrySendsShutdownEventOnFinalStop(t *testing.T) {
	demux, s := newShutdownTelemetryTestDemux(t, "hostname")
	shutdownEventCh := make(chan *event.Event, 1)

	s.On("SendEvents", mock.Anything).Return(nil).Maybe()
	s.On("SendAgentShutdownEvent", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		shutdownEventCh <- args.Get(1).(*event.Event)
	}).Return(nil).Once()

	demux.AddAgentStartupTelemetry("7.0.0")
	demux.Stop()

	var shutdownEvent *event.Event
	select {
	case shutdownEvent = <-shutdownEventCh:
	case <-time.After(time.Second):
		require.FailNow(t, "timed out waiting for Agent Shutdown event")
	}
	require.Equal(t, "Agent Shutdown", shutdownEvent.Title)
	require.Equal(t, "Version 7.0.0", shutdownEvent.Text)
	require.Equal(t, "System", shutdownEvent.SourceTypeName)
	require.Equal(t, "hostname", shutdownEvent.Host)
	require.Equal(t, "Agent Shutdown", shutdownEvent.EventType)

	s.AssertExpectations(t)
}

func newShutdownTelemetryTestDemux(t *testing.T, hostname string) (*AgentDemultiplexer, *MockSerializerIterableSerie) {
	t.Helper()

	deps := createDemultiplexerAgentTestDeps(t)
	demux := InitAndStartAgentDemultiplexer(
		deps.Log,
		NewForwarderTest(deps.Log),
		deps.OrchestratorFwd,
		demuxTestOptions(),
		deps.EventPlatform,
		deps.HaAgent,
		deps.Compressor,
		deps.Tagger,
		deps.FilterList,
		hostname,
	)

	s := &MockSerializerIterableSerie{}
	s.On("AreSeriesEnabled").Return(true).Maybe()
	s.On("AreSketchesEnabled").Return(true).Maybe()
	s.On("SendServiceChecks", mock.Anything).Return(nil).Maybe()

	demux.aggregator.serializer = s
	demux.sharedSerializer = s

	return demux, s
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
	mockConfig.SetInTest("metric_tag_filterlist_adp_only", false)
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

		metric := slices.IndexFunc(s.sketches, func(serie metrics.Distribution) bool {
			return serie.GetName() == "dist.metric"
		})

		require.NotEqualf(-1, metric, "dist.metric not found in %+v", s.sketches)
		tags := strings.Split(s.sketches[metric].(*metrics.SketchSeries).Tags.Join(","), ",")
		require.ElementsMatch(expected, tags)
	}

	// After initial setup, we have filterlist from the configuration file.
	// It may take a little time as it has to be sent to a separate routine.
	require.Eventually(func() bool {
		return len(demux.aggregator.tagFilterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the tagfilterList update")

	// Tag 1 and 2 are excluded
	testCountBlocked([]string{"tag3:three", "tag4:four"}, 32.0)

	// Reset the mock
	s.sketches = []metrics.Distribution{}

	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "exclude",
			Tags:   []string{"tag4", "tag5"},
		}})

	// Ensure the new filter list has been sent.
	require.Eventually(func() bool {
		return len(demux.aggregator.tagFilterListChan) == 0
	}, time.Second, time.Millisecond, "aggregator should consume the tagfilterList update")

	testCountBlocked([]string{"tag1:one", "tag2:two", "tag3:three"}, 62.0)

	demux.Stop()

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

// TestUpdateTagFilterListCheckSamplerCacheInvalidation verifies that when the
// tag filter list is updated, the strip cache on check samplers is cleared so
// that the new include/exclude rules are applied immediately rather than
// returning stale cached contexts until natural expiry.
func TestUpdateTagFilterListCheckSamplerCacheInvalidation(t *testing.T) {
	require := require.New(t)

	mockConfig := configmock.New(t)
	mockConfig.SetInTest("metric_tag_filterlist_adp_only", false)
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

	s := &MockSerializerSketch{}
	s.On("AreSeriesEnabled").Return(true)
	s.On("AreSketchesEnabled").Return(true)
	s.On("SendServiceChecks", mock.Anything).Return(nil)

	demux.aggregator.serializer = s
	demux.sharedSerializer = s

	// Register a check sampler and wait for it to be processed.
	checkID := checkid.ID("test:check:0")
	demux.aggregator.registerSender(checkID)
	require.Eventually(func() bool {
		return len(demux.aggregator.checkItems) == 0
	}, time.Second, time.Millisecond)

	// sendAndFlush submits a distribution sample with the given timestamp via
	// the check sampler path (not DogStatsD), commits it, then flushes.
	// Using an explicit past timestamp ensures the sketch bucket is always
	// older than the commit time and will be flushed.
	sendAndFlush := func(ts float64) {
		demux.aggregator.checkItems <- &senderMetricSample{
			id: checkID,
			metricSample: &metrics.MetricSample{
				Name:       "dist.metric",
				Value:      42,
				Mtype:      metrics.DistributionType,
				Tags:       []string{"tag1:one", "tag2:two", "tag3:three"},
				SampleRate: 1,
				Timestamp:  ts,
			},
		}
		demux.aggregator.checkItems <- &senderMetricSample{
			id:           checkID,
			metricSample: &metrics.MetricSample{},
			commit:       true,
		}
		require.Eventually(func() bool {
			return len(demux.aggregator.checkItems) == 0
		}, time.Second, time.Millisecond)
		demux.ForceFlushToSerializer(time.Now(), true)
	}

	// First send: tag1 and tag2 are excluded. This is a cache miss so the
	// result (only tag3:three) is stored in the strip cache.
	sendAndFlush(1.0)

	idx := slices.IndexFunc(s.sketches, func(ss metrics.Distribution) bool {
		return ss.GetName() == "dist.metric"
	})
	require.NotEqualf(-1, idx, "dist.metric not found in %+v", s.sketches)
	require.ElementsMatch([]string{"tag3:three"}, strings.Split(s.sketches[idx].(*metrics.SketchSeries).Tags.Join(","), ","))

	s.sketches = []metrics.Distribution{}

	// Update the filter list to exclude tag3 instead. SetTagFilterList calls
	// SetAggregatorTagFilterList synchronously, which blocks until the
	// aggregator goroutine has received the new matcher and cleared the caches.
	filterList.SetTagFilterList(map[string]filterlistimpl.MetricTagList{
		"dist.metric": {
			Action: "exclude",
			Tags:   []string{"tag3"},
		}})

	// Second send: the pre-strip key is identical to the first send, so
	// without the fix the stale cache entry would be reused and the sketch
	// would still carry only tag3:three. With the fix the cache was cleared
	// and the new rule is applied, keeping tag1 and tag2.
	sendAndFlush(2.0)

	idx = slices.IndexFunc(s.sketches, func(ss metrics.Distribution) bool {
		return ss.GetName() == "dist.metric"
	})
	require.NotEqualf(-1, idx, "dist.metric not found in %+v", s.sketches)
	require.ElementsMatch([]string{"tag1:one", "tag2:two"}, strings.Split(s.sketches[idx].(*metrics.SketchSeries).Tags.Join(","), ","))

	demux.Stop()
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

	demux.Stop()

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
		defaultforwardermock.MockModule(),
		core.MockBundle(),
		hostnameimpl.MockModule(),
		orchestratormock.MockModule(),
		eventplatformmock.MockModule(),
		logscompression.MockModule(),
		metricscompression.MockModule(),
		haagentmock.Module(),
		filterlistmock.MockModule(),
		fx.Provide(func() tagger.Component { return taggerComponent }),
	)
}
