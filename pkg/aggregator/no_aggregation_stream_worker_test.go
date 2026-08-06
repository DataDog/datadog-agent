// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// TestNoAggStreamWorkerSeriesDisabled is a regression test for a nil pointer
// dereference that occurs when AreSeriesEnabled() returns false. In that case,
// createIterableMetrics returns a nil *IterableSeries, and the worker's
// producer callback was calling w.seriesSink.Append() directly instead of
// using the nil-safe SerieSink parameter provided by Serialize().
func TestNoAggStreamWorkerSeriesDisabled(t *testing.T) {
	noAggWorkerStreamCheckFrequency = 100 * time.Millisecond

	opts := demuxTestOptions()
	opts.NoAggregationPipelineWorkersCount = 1

	mockSerializer := &MockSerializerIterableSerie{}
	mockSerializer.On("AreSeriesEnabled").Return(false)
	mockSerializer.On("AreSketchesEnabled").Return(false)

	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	demux.statsd.noAggStreamWorkers[0].serializer = mockSerializer

	go demux.run()

	batch := testDemuxSamples(t)
	demux.SendSamplesWithoutAggregation(batch)

	// Give time for the worker to process the samples. If the bug is present,
	// the worker goroutine will panic with a nil pointer dereference, crashing
	// the test process.
	time.Sleep(200 * time.Millisecond)
	demux.Stop()
}

// TestNoAggStreamWorkerSampleToSerieFields checks every field the
// no-aggregation pipeline is expected to carry from a MetricSample onto the
// Serie it emits.
//
// Unlike the time sampler, which restores Name/Tags/Host/NoIndex/Interval/Source
// from the context resolver in dedupSerieBySerieSignature, this pipeline builds
// the Serie field by field, so a field is easy to forget here. Each sample below
// carries a distinct Source so that a hardcoded value would not pass.
func TestNoAggStreamWorkerSampleToSerieFields(t *testing.T) {
	require := require.New(t)

	noAggWorkerStreamCheckFrequency = 100 * time.Millisecond

	opts := demuxTestOptions()
	opts.NoAggregationPipelineWorkersCount = 1

	mockSerializer := &MockSerializerIterableSerie{}
	mockSerializer.On("AreSeriesEnabled").Return(true)
	mockSerializer.On("AreSketchesEnabled").Return(true)

	deps := createDemultiplexerAgentTestDeps(t)
	demux := initAgentDemultiplexer(deps.Log, NewForwarderTest(deps.Log), deps.OrchestratorFwd, opts, deps.EventPlatform, deps.HaAgent, deps.Compressor, deps.Tagger, deps.FilterList, "")
	demux.statsd.noAggStreamWorkers[0].serializer = mockSerializer

	go demux.run()

	batch := metrics.MetricSampleBatch{
		metrics.MetricSample{
			Name:      "gauge.metric",
			Host:      "host-gauge",
			Mtype:     metrics.GaugeType,
			Value:     42,
			Timestamp: 1657099120.0,
			Tags:      []string{"tag:1", "tag:2"},
			Source:    metrics.MetricSourceDogstatsd,
		},
		metrics.MetricSample{
			Name:      "counter.metric",
			Host:      "host-counter",
			Mtype:     metrics.CounterType,
			Value:     20,
			Timestamp: 1657099125.0,
			Tags:      []string{"tag:3"},
			Source:    metrics.MetricSourceJmxCustom,
		},
		metrics.MetricSample{
			Name:      "rate.metric",
			Host:      "host-rate",
			Mtype:     metrics.RateType,
			Value:     30,
			Timestamp: 1657099130.0,
			Tags:      []string{"tag:4"},
			Source:    metrics.MetricSourceGPU,
		},
	}

	demux.SendSamplesWithoutAggregation(batch)
	time.Sleep(200 * time.Millisecond) // give some time for the automatic flush to trigger
	demux.Stop()

	// Counters and rates are reported as APIRateType, so their value is divided
	// by the bucket size; gauges are passed through untouched.
	expected := []struct {
		mType metrics.APIMetricType
		value float64
	}{
		{metrics.APIGaugeType, 42},
		{metrics.APIRateType, 2},
		{metrics.APIRateType, 3},
	}

	require.Len(mockSerializer.series, len(batch))
	for i, sample := range batch {
		serie := mockSerializer.series[i]

		require.Equal(sample.Name, serie.Name)
		require.Equal(sample.Host, serie.Host)
		require.Equal(sample.Source, serie.Source, "Source must be copied from the sample: it drives the origin_category of the serialized payload")
		require.Equal(expected[i].mType, serie.MType)
		require.EqualValues(bucketSize, serie.Interval)
		require.ElementsMatch(sample.Tags, serie.Tags.UnsafeToReadOnlySliceString())

		require.Len(serie.Points, 1)
		require.Equal(sample.Timestamp, serie.Points[0].Ts)
		require.Equal(expected[i].value, serie.Points[0].Value)
	}
}

// TestNoAggStreamWorkerSerieFieldsAreAccountedFor fails when a field is added to
// metrics.Serie, so that whoever adds it decides explicitly whether the
// no-aggregation pipeline has to populate it. See
// TestNoAggStreamWorkerSampleToSerieFields for the fields that are asserted, and
// the list below for the ones deliberately left unset.
func TestNoAggStreamWorkerSerieFieldsAreAccountedFor(t *testing.T) {
	// Populated by the no-aggregation pipeline from the sample.
	copied := []string{"Name", "Points", "Tags", "Host", "MType", "Interval", "Source"}

	// Deliberately not populated:
	//   - Device, SourceTypeName, Unit, Resources: not set by the DogStatsD
	//     pipeline at all, aggregated or not.
	//   - NoIndex: never set on DogStatsD samples, so always false.
	//   - ContextKey, NameSuffix: internal to the time sampler's aggregation and
	//     deduplication, meaningless without a context resolver.
	unset := []string{"Device", "SourceTypeName", "Unit", "ContextKey", "NameSuffix", "NoIndex", "Resources"}

	accountedFor := make(map[string]struct{}, len(copied)+len(unset))
	for _, name := range append(append([]string{}, copied...), unset...) {
		accountedFor[name] = struct{}{}
	}

	serieType := reflect.TypeOf(metrics.Serie{})
	for i := 0; i < serieType.NumField(); i++ {
		name := serieType.Field(i).Name
		if _, ok := accountedFor[name]; !ok {
			t.Errorf("metrics.Serie has a new field %q: decide whether the no-aggregation pipeline "+
				"must copy it from the MetricSample, then add it to the copied or unset list here", name)
		}
	}
	require.Equal(t, len(accountedFor), serieType.NumField(), "a field listed here no longer exists on metrics.Serie")
}
