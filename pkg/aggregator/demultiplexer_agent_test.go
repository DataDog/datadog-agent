// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	secretsmock "github.com/DataDog/datadog-agent/comp/core/secrets/mock"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	filterlist "github.com/DataDog/datadog-agent/comp/filterlist/def"
	filterlistmock "github.com/DataDog/datadog-agent/comp/filterlist/fx-mock"
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
	"github.com/DataDog/datadog-agent/pkg/metrics"
	utilstrings "github.com/DataDog/datadog-agent/pkg/util/strings"
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

type DemultiplexerAgentTestDeps struct {
	TestDeps
	OrchestratorFwd orchestratorforwarder.Component
	EventPlatform   eventplatform.Component
	Compressor      compression.Component
	Tagger          tagger.Component
	HaAgent         haagent.Component
	FilterList      filterlist.Component
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

// controllableFilterList is a test implementation of filterlist.Component that allows
// manual control of when filterlist updates are triggered.
type controllableFilterList struct {
	callbacks []func(utilstrings.Matcher, utilstrings.Matcher)
	mu        sync.Mutex
}

func newControllableFilterList() *controllableFilterList {
	return &controllableFilterList{
		callbacks: make([]func(utilstrings.Matcher, utilstrings.Matcher), 0),
	}
}

func (c *controllableFilterList) OnUpdateMetricFilterList(callback func(utilstrings.Matcher, utilstrings.Matcher)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, callback)
	// Call immediately with empty matchers (no filtering initially)
	callback(utilstrings.NewMatcher(nil, false), utilstrings.NewMatcher(nil, false))
}

// TriggerUpdate simulates a filterlist update by calling all registered callbacks
func (c *controllableFilterList) TriggerUpdate(metricNames []string, histoMetricNames []string, matchPrefix bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filterList := utilstrings.NewMatcher(metricNames, matchPrefix)
	histoFilterList := utilstrings.NewMatcher(histoMetricNames, matchPrefix)

	for _, callback := range c.callbacks {
		callback(filterList, histoFilterList)
	}
}

// mockSerializerWithReset extends MockSerializerIterableSerie with a Reset method
type mockSerializerWithReset struct {
	MockSerializerIterableSerie
	mu sync.Mutex
}

func (s *mockSerializerWithReset) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.series = nil
}

func (s *mockSerializerWithReset) SendIterableSeries(seriesSource metrics.SerieSource) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for seriesSource.MoveNext() {
		s.series = append(s.series, seriesSource.Current())
	}
	return nil
}

// TestFilterListCallbackUpdates tests that the demultiplexer correctly handles
// filterlist updates through the OnUpdateMetricFilterList callback mechanism.
// It verifies:
// 1. Initial filterlist callback is invoked immediately upon registration
// 2. Filterlist updates propagate to all workers through their channels
// 3. Multiple rapid updates are handled correctly
// 4. Concurrent updates work without race conditions
func TestFilterListCallbackUpdates(t *testing.T) {
	require := require.New(t)

	// Create controllable filterlist for manual update triggering
	filterList := newControllableFilterList()

	opts := demuxTestOptions()
	deps := createDemultiplexerAgentTestDeps(t)

	// Create demultiplexer with controllable filterlist
	// This will call OnUpdateMetricFilterList which registers the callback
	demux := initAgentDemultiplexer(
		deps.Log,
		NewForwarderTest(deps.Log),
		deps.OrchestratorFwd,
		opts,
		deps.EventPlatform,
		deps.HaAgent,
		deps.Compressor,
		deps.Tagger,
		filterList, // Use our controllable filterlist
		"",
	)

	// Start the demultiplexer (starts all goroutines)
	go demux.run()

	// Give goroutines time to start
	time.Sleep(50 * time.Millisecond)

	// Helper function to drain channels
	drainFilterListChannels := func() {
		// Drain all existing messages from filterListChan
		for _, worker := range demux.statsd.workers {
			select {
			case <-worker.filterListChan:
			default:
			}
		}
		select {
		case <-demux.aggregator.filterListChan:
		default:
		}
	}

	// Test 1: Verify initial callback is called and all workers receive initial filterlist
	t.Run("InitialFilterList", func(t *testing.T) {
		// The callback should have been called during demux initialization
		// and all workers should have received the initial empty filterlist
		// Channels should be empty now after workers processed the initial update
		require.Len(demux.statsd.workers, 1, "Expected 1 worker")

		// Channels should be empty (messages were processed)
		select {
		case <-demux.statsd.workers[0].filterListChan:
			t.Fatal("filterListChan should be empty after initial processing")
		case <-time.After(10 * time.Millisecond):
			// Expected - channel is empty
		}

		select {
		case <-demux.aggregator.filterListChan:
			t.Fatal("aggregator filterListChan should be empty after initial processing")
		case <-time.After(10 * time.Millisecond):
			// Expected - channel is empty
		}
	})

	// Test 2: Update filterlist and verify all workers receive the update
	t.Run("UpdateFilterList", func(t *testing.T) {
		// Drain any existing messages
		drainFilterListChannels()

		// Trigger a filterlist update
		testMetrics := []string{"test.blocked.metric"}
		testHistoMetrics := []string{"test.histo.count"}
		filterList.TriggerUpdate(testMetrics, testHistoMetrics, false)

		// Verify all time sampler workers received the histogram filterlist
		for i, worker := range demux.statsd.workers {
			select {
			case matcher := <-worker.filterListChan:
				// Verify this is the histogram filterlist
				require.True(matcher.Test("test.histo.count"), "Worker %d should have received histogram filterlist", i)
				require.False(matcher.Test("test.blocked.metric"), "Worker %d histogram list shouldn't match non-histo metrics", i)
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Worker %d did not receive filterlist update", i)
			}
		}

		// Verify aggregator received the full filterlist
		select {
		case matcher := <-demux.aggregator.filterListChan:
			require.True(matcher.Test("test.blocked.metric"), "Aggregator should have received full filterlist")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Aggregator did not receive filterlist update")
		}
	})

	// Test 3: Multiple rapid updates
	t.Run("RapidSuccessiveUpdates", func(t *testing.T) {
		drainFilterListChannels()

		// Send rapid filterlist updates
		for i := 0; i < 5; i++ {
			filterList.TriggerUpdate(
				[]string{fmt.Sprintf("metric.%d", i)},
				[]string{fmt.Sprintf("histo.%d.count", i)},
				false,
			)
		}

		// Give channels time to process
		time.Sleep(50 * time.Millisecond)

		// The last update should be in the channels
		// Drain until we get the last one or channel is empty
		var lastWorkerMatcher utilstrings.Matcher
		for {
			select {
			case m := <-demux.statsd.workers[0].filterListChan:
				lastWorkerMatcher = m
			case <-time.After(10 * time.Millisecond):
				// No more messages
				goto workerDone
			}
		}
	workerDone:
		require.NotNil(lastWorkerMatcher, "Should have received at least one update")
		// The last update should be for histo.4.count
		require.True(lastWorkerMatcher.Test("histo.4.count"), "Last worker update should be histo.4.count")
		require.False(lastWorkerMatcher.Test("histo.0.count"), "Should not match old filterlist")

		// Check aggregator
		var lastAggMatcher utilstrings.Matcher
		for {
			select {
			case m := <-demux.aggregator.filterListChan:
				lastAggMatcher = m
			case <-time.After(10 * time.Millisecond):
				goto aggDone
			}
		}
	aggDone:
		require.NotNil(lastAggMatcher, "Aggregator should have received at least one update")
		require.True(lastAggMatcher.Test("metric.4"), "Last aggregator update should be metric.4")
	})

	// Test 4: Concurrent updates from multiple goroutines
	t.Run("ConcurrentUpdates", func(t *testing.T) {
		drainFilterListChannels()

		var wg sync.WaitGroup
		numGoroutines := 10

		// Launch goroutines that trigger updates
		wg.Add(numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			go func(id int) {
				defer wg.Done()
				for j := 0; j < 3; j++ {
					filterList.TriggerUpdate(
						[]string{fmt.Sprintf("concurrent.metric.%d.%d", id, j)},
						[]string{fmt.Sprintf("concurrent.histo.%d.%d.count", id, j)},
						false,
					)
					time.Sleep(5 * time.Millisecond)
				}
			}(i)
		}

		wg.Wait()

		// Give channels time to process all updates
		time.Sleep(100 * time.Millisecond)

		// The important thing is that no panics or data races occurred
		// Drain the channels to verify they're receiving updates
		workerReceived := 0
		for {
			select {
			case <-demux.statsd.workers[0].filterListChan:
				workerReceived++
			case <-time.After(10 * time.Millisecond):
				goto workerCountDone
			}
		}
	workerCountDone:
		require.Greater(workerReceived, 0, "Worker should have received updates")

		aggReceived := 0
		for {
			select {
			case <-demux.aggregator.filterListChan:
				aggReceived++
			case <-time.After(10 * time.Millisecond):
				goto aggCountDone
			}
		}
	aggCountDone:
		require.Greater(aggReceived, 0, "Aggregator should have received updates")
	})

	// Test 5: Empty filterlist (clears filtering)
	t.Run("EmptyFilterList", func(t *testing.T) {
		drainFilterListChannels()

		// Trigger update with empty lists
		filterList.TriggerUpdate(nil, nil, false)

		// Verify workers receive empty matchers
		for i, worker := range demux.statsd.workers {
			select {
			case matcher := <-worker.filterListChan:
				// Empty matcher should not match anything
				require.False(matcher.Test("any.metric"), "Worker %d should have empty matcher", i)
			case <-time.After(100 * time.Millisecond):
				t.Fatalf("Worker %d did not receive empty filterlist", i)
			}
		}

		select {
		case matcher := <-demux.aggregator.filterListChan:
			require.False(matcher.Test("any.metric"), "Aggregator should have empty matcher")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Aggregator did not receive empty filterlist")
		}
	})

	// Test 6: Prefix matching
	t.Run("PrefixMatching", func(t *testing.T) {
		drainFilterListChannels()

		// Update with prefix matching enabled
		filterList.TriggerUpdate(
			[]string{"blocked."},
			[]string{"histo."},
			true, // prefix matching
		)

		// Verify workers receive prefix matcher
		for _, worker := range demux.statsd.workers {
			select {
			case matcher := <-worker.filterListChan:
				require.True(matcher.Test("histo.count"), "Should match prefix")
				require.True(matcher.Test("histo.mean"), "Should match prefix")
				require.False(matcher.Test("other.count"), "Should not match different prefix")
			case <-time.After(100 * time.Millisecond):
				t.Fatal("Worker did not receive filterlist")
			}
		}

		select {
		case matcher := <-demux.aggregator.filterListChan:
			require.True(matcher.Test("blocked.metric"), "Should match prefix")
			require.True(matcher.Test("blocked.another"), "Should match prefix")
			require.False(matcher.Test("allowed.metric"), "Should not match different prefix")
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Aggregator did not receive filterlist")
		}
	})

	demux.Stop(true)
}
