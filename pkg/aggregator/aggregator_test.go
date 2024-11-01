// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	// stdlib
	"errors"
	"expvar"
	"fmt"
	"sort"
	"testing"
	"time"

	// 3p
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/tagger"
	"github.com/DataDog/datadog-agent/comp/core/tagger/taggerimpl"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	orchestratorforwarder "github.com/DataDog/datadog-agent/comp/forwarder/orchestrator"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	serializermock "github.com/DataDog/datadog-agent/pkg/serializer/mocks"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

var (
	checkID1 checkid.ID = "1"
	checkID2 checkid.ID = "2"
)

const defaultHostname = "hostname"

func init() {
	initF()
}

func initF() {
	recurrentSeries = metrics.Series{}
	tagsetTlm.reset()
}

func testNewFlushTrigger(start time.Time, waitForSerializer bool) flushTrigger {
	seriesSink := metrics.NewIterableSeries(func(_ *metrics.Serie) {}, 1000, 1000)
	flushedSketches := make(metrics.SketchSeriesList, 0)

	return flushTrigger{
		trigger: trigger{
			time:              start,
			blockChan:         nil,
			waitForSerializer: waitForSerializer,
		},
		sketchesSink: &flushedSketches,
		seriesSink:   seriesSink,
	}
}

func getAggregator(t *testing.T) *BufferedAggregator {
	deps := createAggrDeps(t)
	deps.Demultiplexer.Aggregator().tlmContainerTagsEnabled = false // do not use a ContainerImpl
	return deps.Demultiplexer.Aggregator()
}

func TestRegisterCheckSampler(t *testing.T) {
	// this test IS USING globals
	// -

	agg := getAggregator(t)
	agg.checkSamplers = make(map[checkid.ID]*CheckSampler)

	lenSenders := func(n int) bool {
		agg.mu.Lock()
		defer agg.mu.Unlock()
		return len(agg.checkSamplers) == n
	}

	err := agg.registerSender(checkID1)
	assert.Nil(t, err)

	require.Eventually(t, func() bool { return lenSenders(1) }, time.Second, 10*time.Millisecond)

	err = agg.registerSender(checkID2)
	assert.Nil(t, err)
	require.Eventually(t, func() bool { return lenSenders(2) }, time.Second, 10*time.Millisecond)
}

func TestDeregisterCheckSampler(t *testing.T) {
	// this test IS USING globals
	// -

	deps := createAggrDeps(t)
	demux := deps.Demultiplexer

	defer demux.Stop(false)

	agg := demux.Aggregator()
	agg.checkSamplers = make(map[checkid.ID]*CheckSampler)

	agg.registerSender(checkID1)
	agg.registerSender(checkID2)

	require.Eventually(t, func() bool {
		agg.mu.Lock()
		defer agg.mu.Unlock()
		return len(agg.checkSamplers) == 2
	}, time.Second, 10*time.Millisecond)

	agg.deregisterSender(checkID1)

	require.Eventually(t, func() bool {
		agg.mu.Lock()
		defer agg.mu.Unlock()
		return agg.checkSamplers[checkID1].deregistered && !agg.checkSamplers[checkID2].deregistered
	}, time.Second, 10*time.Millisecond)

	agg.Flush(testNewFlushTrigger(time.Now(), false))

	agg.mu.Lock()
	require.Len(t, agg.checkSamplers, 1)
	_, ok := agg.checkSamplers[checkID1]
	assert.False(t, ok)
	_, ok = agg.checkSamplers[checkID2]
	assert.True(t, ok)
	agg.mu.Unlock()
}

func TestAddServiceCheckDefaultValues(t *testing.T) {
	// this test is not using anything global
	// -

	s := &MockSerializerIterableSerie{}
	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())
	agg := NewBufferedAggregator(s, nil, taggerComponent, "resolved-hostname", DefaultFlushInterval)

	agg.addServiceCheck(servicecheck.ServiceCheck{
		// leave Host and Ts fields blank
		CheckName: "my_service.can_connect",
		Status:    servicecheck.ServiceCheckOK,
		Tags:      []string{"bar", "foo", "bar"},
		Message:   "message",
	})
	agg.addServiceCheck(servicecheck.ServiceCheck{
		CheckName: "my_service.can_connect",
		Status:    servicecheck.ServiceCheckOK,
		Host:      "my-hostname",
		Tags:      []string{"foo", "foo", "bar"},
		Ts:        12345,
		Message:   "message",
	})

	require.Len(t, agg.serviceChecks, 2)
	assert.Equal(t, "", agg.serviceChecks[0].Host)
	assert.ElementsMatch(t, []string{"bar", "foo"}, agg.serviceChecks[0].Tags)
	assert.NotZero(t, agg.serviceChecks[0].Ts) // should be set to the current time, let's just check that it's not 0
	assert.Equal(t, "my-hostname", agg.serviceChecks[1].Host)
	assert.ElementsMatch(t, []string{"foo", "bar"}, agg.serviceChecks[1].Tags)
	assert.Equal(t, int64(12345), agg.serviceChecks[1].Ts)
}

func TestAddEventDefaultValues(t *testing.T) {
	// this test is not using anything global
	// -

	s := &MockSerializerIterableSerie{}
	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())
	agg := NewBufferedAggregator(s, nil, taggerComponent, "resolved-hostname", DefaultFlushInterval)

	agg.addEvent(event.Event{
		// only populate required fields
		Title: "An event occurred",
		Text:  "Event description",
	})
	agg.addEvent(event.Event{
		// populate all fields
		Title:          "Another event occurred",
		Text:           "Other event description",
		Ts:             12345,
		Priority:       event.PriorityNormal,
		Host:           "my-hostname",
		Tags:           []string{"foo", "bar", "foo"},
		AlertType:      event.AlertTypeError,
		AggregationKey: "my_agg_key",
		SourceTypeName: "custom_source_type",
	})

	require.Len(t, agg.events, 2)
	// Default values are set on Ts
	event1 := agg.events[0]
	assert.Equal(t, "An event occurred", event1.Title)
	assert.Equal(t, "", event1.Host)
	assert.NotZero(t, event1.Ts) // should be set to the current time, let's just check that it's not 0
	assert.Zero(t, event1.Priority)
	assert.Zero(t, event1.Tags)
	assert.Zero(t, event1.AlertType)
	assert.Zero(t, event1.AggregationKey)
	assert.Zero(t, event1.SourceTypeName)

	event2 := agg.events[1]
	// No change is made
	assert.Equal(t, "Another event occurred", event2.Title)
	assert.Equal(t, "my-hostname", event2.Host)
	assert.Equal(t, int64(12345), event2.Ts)
	assert.Equal(t, event.PriorityNormal, event2.Priority)
	assert.ElementsMatch(t, []string{"foo", "bar"}, event2.Tags)
	assert.Equal(t, event.AlertTypeError, event2.AlertType)
	assert.Equal(t, "my_agg_key", event2.AggregationKey)
	assert.Equal(t, "custom_source_type", event2.SourceTypeName)
}

func TestDefaultData(t *testing.T) {
	// this test IS USING globals (tagsetTlm) but a local aggregator
	// -

	s := &MockSerializerIterableSerie{}
	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())
	agg := NewBufferedAggregator(s, nil, taggerComponent, "hostname", DefaultFlushInterval)

	start := time.Now()

	// Check only the name for `datadog.agent.up` as the timestamp may not be the same.
	agentUpMatcher := mock.MatchedBy(func(m servicecheck.ServiceChecks) bool {
		require.Equal(t, 1, len(m))
		require.Equal(t, "datadog.agent.up", m[0].CheckName)
		require.Equal(t, servicecheck.ServiceCheckOK, m[0].Status)
		require.Equal(t, []string{}, m[0].Tags)
		require.Equal(t, agg.hostname, m[0].Host)

		return true
	})
	s.On("SendServiceChecks", agentUpMatcher).Return(nil).Times(1)

	series := metrics.Series{&metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           tagset.CompositeTagsFromSlice([]string{fmt.Sprintf("version:%s", version.AgentVersion)}),
		Host:           agg.hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 0, Ts: float64(start.Unix())}},
		Host:           agg.hostname,
		Tags:           tagset.CompositeTagsFromSlice([]string{}),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
		NoIndex:        true,
	}}

	s.On("SendSeries", series).Return(nil).Times(1)

	agg.Flush(testNewFlushTrigger(start, false))
	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")

	// not counted as huge for (just checking the first threshold..)
	assert.Equal(t, uint64(0), tagsetTlm.hugeSeriesCount[0].Load())
}

func TestSeriesTooManyTags(t *testing.T) {
	// this test IS USING globals (tagsetTlm and recurrentSeries) but a local aggregator
	// -

	test := func(tagCount int) func(t *testing.T) {
		expHugeCounts := make([]uint64, tagsetTlm.size)

		for i, thresh := range tagsetTlm.sizeThresholds {
			if uint64(tagCount) > thresh {
				expHugeCounts[i]++
			}
		}

		return func(t *testing.T) {
			s := &MockSerializerIterableSerie{}
			deps := createAggrDeps(t)
			demux := deps.Demultiplexer

			demux.sharedSerializer = s
			demux.aggregator.serializer = s

			start := time.Now()

			var tags []string
			for i := 0; i < tagCount; i++ {
				tags = append(tags, fmt.Sprintf("tag%d", i))
			}

			ser := &metrics.Serie{
				Name:           "test.series",
				Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
				Tags:           tagset.CompositeTagsFromSlice(tags),
				Host:           demux.Aggregator().hostname,
				MType:          metrics.APIGaugeType,
				SourceTypeName: "System",
			}
			AddRecurrentSeries(ser)

			s.On("AreSeriesEnabled").Return(true)
			s.On("AreSketchesEnabled").Return(true)
			s.On("SendServiceChecks", mock.Anything).Return(nil).Times(1)
			s.On("SendIterableSeries", mock.Anything).Return(nil).Times(1)

			demux.ForceFlushToSerializer(start, true)
			s.AssertNotCalled(t, "SendEvents")
			s.AssertNotCalled(t, "SendSketch")

			expMap := map[string]uint64{}
			for i, thresh := range tagsetTlm.sizeThresholds {
				assert.Equal(t, expHugeCounts[i], tagsetTlm.hugeSeriesCount[i].Load())
				expMap[fmt.Sprintf("Above%d", thresh)] = expHugeCounts[i]
			}
			gotMap := aggregatorExpvars.Get("MetricTags").(expvar.Func).Value().(map[string]map[string]uint64)["Series"]
			assert.Equal(t, expMap, gotMap)

			// reset telemetry for next tests
			demux.Stop(false)
			recurrentSeries = metrics.Series{}
			tagsetTlm.reset()
		}
	}
	t.Run("not-huge", test(10))
	t.Run("almost-huge", test(95))
	t.Run("huge", test(110))
}

func TestDistributionsTooManyTags(t *testing.T) {
	// this test IS USING globals (tagsetTlm and recurrentSeries) but a local aggregator
	// -

	test := func(tagCount int) func(t *testing.T) {
		expHugeCounts := make([]uint64, tagsetTlm.size)

		for i, thresh := range tagsetTlm.sizeThresholds {
			if uint64(tagCount) > thresh {
				expHugeCounts[i]++
			}
		}

		return func(t *testing.T) {
			s := &MockSerializerIterableSerie{}
			deps := createAggrDeps(t)
			demux := deps.Demultiplexer

			demux.sharedSerializer = s
			demux.aggregator.serializer = s

			start := time.Now()

			var tags []string
			for i := 0; i < tagCount; i++ {
				tags = append(tags, fmt.Sprintf("tag%d", i))
			}

			samp := metrics.MetricSample{
				Name:      "test.sample",
				Value:     13.0,
				Mtype:     metrics.DistributionType,
				Tags:      tags,
				Host:      "",
				Timestamp: timeNowNano() - 10000000,
			}
			demux.AggregateSample(samp)

			time.Sleep(1 * time.Second)

			s.On("AreSeriesEnabled").Return(true)
			s.On("AreSketchesEnabled").Return(true)
			s.On("SendServiceChecks", mock.Anything).Return(nil).Times(1)
			s.On("SendIterableSeries", mock.Anything).Return(nil).Times(1)
			s.On("SendSketch", mock.Anything).Return(nil).Times(1)

			demux.ForceFlushToSerializer(start, true)
			s.AssertNotCalled(t, "SendEvents")

			expMap := map[string]uint64{}
			for i, thresh := range tagsetTlm.sizeThresholds {
				assert.Equal(t, expHugeCounts[i], tagsetTlm.hugeSketchesCount[i].Load())
				expMap[fmt.Sprintf("Above%d", thresh)] = expHugeCounts[i]
			}
			gotMap := aggregatorExpvars.Get("MetricTags").(expvar.Func).Value().(map[string]map[string]uint64)["Sketches"]
			assert.Equal(t, expMap, gotMap)

			// reset for next tests
			recurrentSeries = metrics.Series{}
			tagsetTlm.reset()
		}
	}
	t.Run("not-huge", test(10))
	t.Run("almost-huge", test(95))
	t.Run("huge", test(110))
}

func TestRecurrentSeries(t *testing.T) {
	// this test IS USING globals (recurrentSeries)
	// -

	s := &MockSerializerIterableSerie{}
	s.On("AreSeriesEnabled").Return(true)
	s.On("AreSketchesEnabled").Return(true)
	deps := createAggrDeps(t)
	demux := deps.Demultiplexer

	demux.aggregator.serializer = s
	demux.sharedSerializer = s

	// Add two recurrentSeries
	AddRecurrentSeries(&metrics.Serie{
		Name:   "some.metric.1",
		Points: []metrics.Point{{Value: 21}},
		Tags:   tagset.CompositeTagsFromSlice([]string{"tag:1", "tag:2"}),
		MType:  metrics.APIGaugeType,
	})
	AddRecurrentSeries(&metrics.Serie{
		Name:           "some.metric.2",
		Points:         []metrics.Point{{Value: 22}},
		Tags:           tagset.CompositeTagsFromSlice([]string{}),
		Host:           "non default host",
		MType:          metrics.APIGaugeType,
		SourceTypeName: "non default SourceTypeName",
	})

	start := time.Now()

	expectedSeries := metrics.Series{&metrics.Serie{
		Name:           "some.metric.1",
		Points:         []metrics.Point{{Value: 21, Ts: float64(start.Unix())}},
		Tags:           tagset.NewCompositeTags([]string{"tag:1", "tag:2"}, []string{}),
		Host:           demux.Aggregator().hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           "some.metric.2",
		Points:         []metrics.Point{{Value: 22, Ts: float64(start.Unix())}},
		Tags:           tagset.NewCompositeTags([]string{}, []string{}),
		Host:           "non default host",
		MType:          metrics.APIGaugeType,
		SourceTypeName: "non default SourceTypeName",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("datadog.%s.running", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 1, Ts: float64(start.Unix())}},
		Tags:           tagset.CompositeTagsFromSlice([]string{fmt.Sprintf("version:%s", version.AgentVersion)}),
		Host:           demux.Aggregator().hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
	}, &metrics.Serie{
		Name:           fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", flavor.GetFlavor()),
		Points:         []metrics.Point{{Value: 0, Ts: float64(start.Unix())}},
		Host:           demux.Aggregator().hostname,
		Tags:           tagset.CompositeTagsFromSlice([]string{}),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "System",
		NoIndex:        true,
	}}

	// Check only the name for `datadog.agent.up` as the timestamp may not be the same.
	agentUpMatcher := mock.MatchedBy(func(m servicecheck.ServiceChecks) bool {
		require.Equal(t, 1, len(m))
		require.Equal(t, "datadog.agent.up", m[0].CheckName)
		require.Equal(t, servicecheck.ServiceCheckOK, m[0].Status)
		require.Equal(t, []string{}, m[0].Tags)
		require.Equal(t, demux.Aggregator().hostname, m[0].Host)

		return true
	})

	s.On("SendServiceChecks", agentUpMatcher).Return(nil).Times(1)
	demux.ForceFlushToSerializer(start, true)
	require.EqualValues(t, expectedSeries, s.series)
	s.series = nil

	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")

	// Assert that recurrentSeries are sent on each flushed
	// same goes for the service check
	s.On("SendServiceChecks", agentUpMatcher).Return(nil).Times(1)
	demux.ForceFlushToSerializer(start, true)
	require.EqualValues(t, expectedSeries, s.series)
	s.series = nil

	s.AssertNotCalled(t, "SendEvents")
	s.AssertNotCalled(t, "SendSketch")
	time.Sleep(1 * time.Second) // a lot of async thing are going on
	s.AssertExpectations(t)

	recurrentSeries = nil
}

func TestTags(t *testing.T) {
	// this test is not using anything global
	// -

	tests := []struct {
		name                    string
		hostname                string
		tlmContainerTagsEnabled bool
		agentTags               func(types.TagCardinality) ([]string, error)
		globalTags              func(types.TagCardinality) ([]string, error)
		withVersion             bool
		want                    []string
	}{
		{
			name:                    "tags disabled, with version",
			hostname:                "hostname",
			tlmContainerTagsEnabled: false,
			agentTags:               func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			globalTags:              func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             true,
			want:                    []string{"version:" + version.AgentVersion},
		},
		{
			name:                    "tags disabled, without version",
			hostname:                "hostname",
			tlmContainerTagsEnabled: false,
			agentTags:               func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			globalTags:              func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             false,
			want:                    []string{},
		},
		{
			name:                    "tags enabled, with version",
			hostname:                "hostname",
			tlmContainerTagsEnabled: true,
			agentTags:               func(types.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			globalTags:              func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             true,
			want:                    []string{"container_name:agent", "version:" + version.AgentVersion},
		},
		{
			name:                    "tags enabled, without version",
			hostname:                "hostname",
			tlmContainerTagsEnabled: true,
			agentTags:               func(types.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			globalTags:              func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             false,
			want:                    []string{"container_name:agent"},
		},
		{
			name:                    "tags enabled, with version, tagger error",
			hostname:                "hostname",
			tlmContainerTagsEnabled: true,
			agentTags:               func(types.TagCardinality) ([]string, error) { return nil, errors.New("no tags") },
			globalTags:              func(types.TagCardinality) ([]string, error) { return nil, errors.New("disabled") },
			withVersion:             true,
			want:                    []string{"version:" + version.AgentVersion},
		},
		{
			name:                    "tags enabled, with version, with global tags (no hostname)",
			hostname:                "",
			tlmContainerTagsEnabled: true,
			agentTags:               func(types.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			globalTags:              func(types.TagCardinality) ([]string, error) { return []string{"kube_cluster_name:foo"}, nil },
			withVersion:             true,
			want:                    []string{"container_name:agent", "version:" + version.AgentVersion, "kube_cluster_name:foo"},
		},
		{
			name:                    "tags enabled, with version, with global tags (hostname present)",
			hostname:                "hostname",
			tlmContainerTagsEnabled: true,
			agentTags:               func(types.TagCardinality) ([]string, error) { return []string{"container_name:agent"}, nil },
			globalTags:              func(types.TagCardinality) ([]string, error) { return []string{"kube_cluster_name:foo"}, nil },
			withVersion:             true,
			want:                    []string{"container_name:agent", "version:" + version.AgentVersion, "kube_cluster_name:foo"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("basic_telemetry_add_container_tags", tt.tlmContainerTagsEnabled)

			taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())

			agg := NewBufferedAggregator(nil, nil, taggerComponent, tt.hostname, time.Second)
			agg.agentTags = tt.agentTags
			agg.globalTags = tt.globalTags
			assert.ElementsMatch(t, tt.want, agg.tags(tt.withVersion))
		})
	}
}

func TestTimeSamplerFlush(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("dogstatsd_pipeline_count", 1)

	s := &MockSerializerIterableSerie{}
	s.On("AreSeriesEnabled").Return(true)
	s.On("AreSketchesEnabled").Return(true)
	s.On("SendServiceChecks", mock.Anything).Return(nil)
	deps := createAggrDeps(t)
	demux := deps.Demultiplexer

	demux.aggregator.serializer = s
	demux.sharedSerializer = s
	expectedSeries := flushSomeSamples(demux)
	assertSeriesEqual(t, s.series, expectedSeries)
}

func TestAddDJMRecurrentSeries(t *testing.T) {
	// this test IS USING globals (recurrentSeries)
	// -
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("djm_config.enabled", true)

	s := &MockSerializerIterableSerie{}
	// NewBufferedAggregator with DJM enable will create a new recurrentSeries
	taggerComponent := fxutil.Test[tagger.Mock](t, taggerimpl.MockModule())
	NewBufferedAggregator(s, nil, taggerComponent, "hostname", DefaultFlushInterval)

	expectedRecurrentSeries := metrics.Series{&metrics.Serie{
		Name:   "datadog.djm.agent_host",
		Points: []metrics.Point{{Value: 1.0}},
		MType:  metrics.APIGaugeType,
	}}

	require.EqualValues(t, expectedRecurrentSeries, recurrentSeries)

	// Reset recurrentSeries
	recurrentSeries = metrics.Series{}
}

// The implementation of MockSerializer.SendIterableSeries uses `s.Called(series).Error(0)`.
// It calls internaly `Printf` on each field of the real type of `IterableStreamJSONMarshaler` which is `IterableSeries`.
// It can lead to a race condition, if another goruntine call `IterableSeries.Append` which modifies `series.count`.
// MockSerializerIterableSerie overrides `SendIterableSeries` to avoid this issue.
// It also overrides `SendSeries` for simplificy.
type MockSerializerIterableSerie struct {
	series []*metrics.Serie
	serializermock.MetricSerializer
}

func (s *MockSerializerIterableSerie) SendIterableSeries(seriesSource metrics.SerieSource) error {
	for seriesSource.MoveNext() {
		s.series = append(s.series, seriesSource.Current())
	}
	return nil
}

func flushSomeSamples(demux *AgentDemultiplexer) map[string]*metrics.Serie {
	timeSamplerBucketSize := float64(10)
	timestamps := []float64{10, 10 + timeSamplerBucketSize}
	sampleCount := 100
	expectedSeries := make(map[string]*metrics.Serie)

	for v, timestamp := range timestamps {
		value := float64(v + 1)
		for i := 0; i < sampleCount; i++ {
			name := fmt.Sprintf("serie%d", i)

			demux.AggregateSample(metrics.MetricSample{Name: name, Value: value, Mtype: metrics.CountType, Timestamp: timestamp})

			if _, found := expectedSeries[name]; !found {
				expectedSeries[name] = &metrics.Serie{
					Name:     name,
					MType:    metrics.APICountType,
					Interval: int64(10),
					Tags:     tagset.NewCompositeTags([]string{}, []string{}),
				}
			}
			expectedSeries[name].Points = append(expectedSeries[name].Points, metrics.Point{Ts: timestamp, Value: value})
		}
	}

	// we have to wait here because AggregateSample is async and we want to be
	// sure all samples have been processed by the sampler
	time.Sleep(1 * time.Second)

	demux.ForceFlushToSerializer(time.Unix(int64(timeSamplerBucketSize)*3, 0), true)
	return expectedSeries
}

func assertSeriesEqual(t *testing.T, series []*metrics.Serie, expectedSeries map[string]*metrics.Serie) {
	// default series

	r := require.New(t)
	for _, serie := range series {
		// ignore default series automatically sent by the aggregator
		if serie.Name == fmt.Sprintf("datadog.%s.running", flavor.GetFlavor()) ||
			serie.Name == fmt.Sprintf("n_o_i_n_d_e_x.datadog.%s.payload.dropped", flavor.GetFlavor()) {
			// ignore default series
			continue
		}

		expected, found := expectedSeries[serie.Name]

		delete(expectedSeries, serie.Name)
		if !found {
			t.Fatalf("Cannot find serie: %s", serie.Name)
		}
		if expected == nil {
			// default series
			continue
		}
		// ignore context key
		expected.ContextKey = serie.ContextKey

		sort.Slice(serie.Points, func(i int, j int) bool {
			return serie.Points[i].Ts < serie.Points[j].Ts
		})
		sort.Slice(expected.Points, func(i int, j int) bool {
			return expected.Points[i].Ts < expected.Points[j].Ts
		})
		r.EqualValues(expected, serie)
	}

	r.Empty(expectedSeries)
}

type aggregatorDeps struct {
	TestDeps
	Demultiplexer    *AgentDemultiplexer
	OrchestratorFwd  orchestratorforwarder.Component
	EventPlatformFwd eventplatform.Component
}

func createAggrDeps(t *testing.T) aggregatorDeps {
	deps := fxutil.Test[TestDeps](t, defaultforwarder.MockModule(), core.MockBundle(), compressionimpl.MockModule())

	opts := demuxTestOptions()
	return aggregatorDeps{
		TestDeps:      deps,
		Demultiplexer: InitAndStartAgentDemultiplexerForTest(deps, opts, ""),
	}
}

func TestStatsCopy(t *testing.T) {
	// Flushes    [32]int64 // circular buffer of recent flushes stat
	// FlushIndex int       // last flush position in circular buffer
	// LastFlush  int64     // most recent flush stat, provided for convenience
	// Name       string

	stats := &Stats{
		Flushes:    [32]int64{1, 2, 3},
		FlushIndex: 2,
		LastFlush:  1,
		Name:       "name",
	}
	stats.Flushes[31] = 32

	statsCopy := stats.copy()
	assert.Equal(t, stats.Flushes, statsCopy.Flushes)
	assert.Equal(t, stats.FlushIndex, statsCopy.FlushIndex)
	assert.Equal(t, stats.LastFlush, statsCopy.LastFlush)
	assert.Equal(t, stats.Name, statsCopy.Name)
}
