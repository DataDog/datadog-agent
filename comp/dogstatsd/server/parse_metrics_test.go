// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
)

func parseMetricSample(t *testing.T, overrides map[string]any, rawSample []byte) (dogstatsdMetricSample, error) {
	deps := newServerDeps(t, fx.Replace(config.MockParams{Overrides: overrides}))

	p := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	_, found := overrides["parser"]
	if found {
		p = overrides["parser"].(*parser)
	}

	return p.parseMetricSample(rawSample)
}

const epsilon = 0.00001

func TestParseGauge(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Equal(t, 666.0, sample.value)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666:777|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Len(t, sample.values, 2)
	assert.InEpsilon(t, 666.0, sample.values[0], epsilon)
	assert.InEpsilon(t, 777.0, sample.values[1], epsilon)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseCounter(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:21|c"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 21.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, countType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseCounterMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666:777|c"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Len(t, sample.values, 2)
	assert.InEpsilon(t, 666.0, sample.values[0], epsilon)
	assert.InEpsilon(t, 777.0, sample.values[1], epsilon)
	assert.Equal(t, countType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseCounterWithTags(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("custom_counter:1|c|#protocol:http,bench"))

	assert.NoError(t, err)

	assert.Equal(t, "custom_counter", sample.name)
	assert.InEpsilon(t, 1.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, countType, sample.metricType)
	assert.Equal(t, 2, len(sample.tags))
	assert.Equal(t, "protocol:http", sample.tags[0])
	assert.Equal(t, "bench", sample.tags[1])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseHistogram(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:21|h"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 21.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, histogramType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseHistogramrMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:21:22|h"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Len(t, sample.values, 2)
	assert.InEpsilon(t, 21.0, sample.values[0], epsilon)
	assert.InEpsilon(t, 22.0, sample.values[1], epsilon)
	assert.Equal(t, histogramType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseTimer(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:21|ms"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 21.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, timingType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseTimerMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:21:22|ms"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Len(t, sample.values, 2)
	assert.InEpsilon(t, 21.0, sample.values[0], epsilon)
	assert.InEpsilon(t, 22.0, sample.values[1], epsilon)
	assert.Equal(t, timingType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseSet(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:abc|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Equal(t, "abc", sample.setValue)
	assert.Equal(t, setType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseSetMultiple(t *testing.T) {
	// multiple values are not supported for set. ':' can be part of the
	// set value for backward compatibility
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:abc:def|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Equal(t, "abc:def", sample.setValue)
	assert.Equal(t, setType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestSampleDistribution(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:3.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 3.5, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, distributionType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.Zero(t, sample.ts)
}

func TestParseDistributionMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:3.5:4.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Len(t, sample.values, 2)
	assert.InEpsilon(t, 3.5, sample.values[0], epsilon)
	assert.InEpsilon(t, 4.5, sample.values[1], epsilon)
	assert.Equal(t, distributionType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.Zero(t, sample.ts)
}

func TestParseSetUnicode(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:♬†øU†øU¥ºuT0♪|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", sample.setValue)
	assert.Equal(t, setType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeWithTags(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 2, len(sample.tags))
	assert.Equal(t, "sometag1:somevalue1", sample.tags[0])
	assert.Equal(t, "sometag2:somevalue2", sample.tags[1])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeWithNoTags(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666|g"))
	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Empty(t, sample.tags)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeWithSampleRate(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666|g|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeWithPoundOnly(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666|g|#"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	assert.Len(t, sample.tags, 0)
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseGaugeWithUnicode(t *testing.T) {
	sample, err := parseMetricSample(t, make(map[string]any), []byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", sample.name)
	assert.InEpsilon(t, 666.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "intitulé:T0µ", sample.tags[0])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)
}

func TestParseMetricError(t *testing.T) {
	// not enough information
	_, err := parseMetricSample(t, make(map[string]any), []byte("daemon:666"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:666|"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:|g"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, make(map[string]any), []byte(":666|g"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, make(map[string]any), []byte("abc666|g"))
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:666|g|m:test"))
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:abc|g"))
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:666|unknown"))
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricSample(t, make(map[string]any), []byte("daemon:666|g|@abc"))
	assert.Error(t, err)
}

func TestParseGaugeWithTimestamp(t *testing.T) {
	// disable the no agg pipeline

	cfg := map[string]any{}
	cfg["dogstatsd_no_aggregation_pipeline"] = false

	// no timestamp should be read when the no agg pipeline is off

	sample, err := parseMetricSample(t, cfg, []byte("metric:1234|g|#onetag|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "onetag", sample.tags[0])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Zero(t, sample.ts)

	// re-enable the no aggregation pipeline

	cfg["dogstatsd_no_aggregation_pipeline"] = true

	// with tags and timestamp

	sample, err = parseMetricSample(t, cfg, []byte("metric:1234|g|#onetag|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "onetag", sample.tags[0])
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100430, 0))

	// with weird tags field and timestamp

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|#|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100430, 0))

	// with sample rate and timestamp

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|@0.21|T1657100440"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 0, len(sample.tags))
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100440, 0))

	// with tags, sample rate and timestamp

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|#thereisatag|@0.21|T1657100440"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "thereisatag", sample.tags[0])
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100440, 0))

	// varying the order of the tags, sample rate and timestamp entries

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|#thereisatag|T1657100540|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "thereisatag", sample.tags[0])
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|@0.21|T1657100540|#thereisatag"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "thereisatag", sample.tags[0])
	assert.InEpsilon(t, 0.21, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|T1657100540|@0.25|#atag"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "atag", sample.tags[0])
	assert.InEpsilon(t, 0.25, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, make(map[string]any), []byte("metric:1234|g|T1657100540|#atag|@0.25"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.name)
	assert.InEpsilon(t, 1234.0, sample.value, epsilon)
	require.Nil(t, sample.values)
	assert.Equal(t, gaugeType, sample.metricType)
	require.Equal(t, 1, len(sample.tags))
	assert.Equal(t, "atag", sample.tags[0])
	assert.InEpsilon(t, 0.25, sample.sampleRate, epsilon)
	assert.Equal(t, sample.ts, time.Unix(1657100540, 0))
}

func TestParseGaugeTimestampMalformed(t *testing.T) {
	// enable the no aggregation pipeline

	cfg := map[string]any{}
	cfg["dogstatsd_no_aggregation_pipeline"] = true

	// bad value
	_, err := parseMetricSample(t, cfg, []byte("metric:1234|g|#onetag|TABCD"))
	assert.Error(t, err)

	// no value
	_, err = parseMetricSample(t, cfg, []byte("metric:1234|g|#onetag|T"))
	assert.Error(t, err)

	// negative value
	_, err = parseMetricSample(t, cfg, []byte("metric:1234|g|#onetag|T-102348932"))
	assert.Error(t, err)
}

func TestParseManyPipes(t *testing.T) {
	t.Run("Sample rate and container ID (4 pipes)", func(t *testing.T) {
		sample, err := parseMetricSample(t, make(map[string]any), []byte("example.metric:2.39283|d|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.name)
		assert.InEpsilon(t, 2.39283, sample.value, epsilon)
		require.Nil(t, sample.values)
		assert.Equal(t, distributionType, sample.metricType)
		require.Equal(t, 1, len(sample.tags))
		assert.Equal(t, "environment:dev", sample.tags[0])
		assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp (5 pipes)", func(t *testing.T) {
		cfg := map[string]any{}
		cfg["dogstatsd_no_aggregation_pipeline"] = true

		sample, err := parseMetricSample(t, cfg, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.name)
		assert.InEpsilon(t, 2.39283, sample.value, epsilon)
		require.Nil(t, sample.values)
		assert.Equal(t, distributionType, sample.metricType)
		require.Equal(t, 1, len(sample.tags))
		assert.Equal(t, sample.ts, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.tags[0])
		assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp and future extension (6 pipes)", func(t *testing.T) {

		cfg := map[string]any{}
		cfg["dogstatsd_no_aggregation_pipeline"] = true

		sample, err := parseMetricSample(t, cfg, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.name)
		assert.InEpsilon(t, 2.39283, sample.value, epsilon)
		require.Nil(t, sample.values)
		assert.Equal(t, distributionType, sample.metricType)
		require.Equal(t, 1, len(sample.tags))
		assert.Equal(t, sample.ts, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.tags[0])
		assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp and 2 future extensions (7 pipes)", func(t *testing.T) {
		cfg := map[string]any{}
		cfg["dogstatsd_no_aggregation_pipeline"] = true

		sample, err := parseMetricSample(t, cfg, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature|f2:omgthisoneiscooler"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.name)
		assert.InEpsilon(t, 2.39283, sample.value, epsilon)
		require.Nil(t, sample.values)
		assert.Equal(t, distributionType, sample.metricType)
		require.Equal(t, 1, len(sample.tags))
		assert.Equal(t, sample.ts, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.tags[0])
		assert.InEpsilon(t, 1.0, sample.sampleRate, epsilon)
	})
}

func TestParseContainerID(t *testing.T) {
	cfg := map[string]any{}
	cfg["dogstatsd_origin_detection_client"] = true

	// Testing with a container ID
	sample, err := parseMetricSample(t, cfg, []byte("metric:1234|g|c:1234567890abcdef"))
	require.NoError(t, err)
	assert.Equal(t, []byte("1234567890abcdef"), sample.containerID)

	// Testing with an Inode
	deps := newServerDeps(t, fx.Replace(config.MockParams{Overrides: cfg}))
	p := newParser(deps.Config, newFloat64ListPool(), 1, deps.WMeta)
	mockProvider := mock.NewMetricsProvider()
	mockProvider.RegisterMetaCollector(&mock.MetaCollector{
		CIDFromInode: map[uint64]string{
			1234567890: "1234567890abcdef",
		},
	})
	p.provider = mockProvider
	cfg["parser"] = p

	sample, err = parseMetricSample(t, cfg, []byte("metric:1234|g|c:in-1234567890"))
	require.NoError(t, err)
	assert.Equal(t, []byte("1234567890abcdef"), sample.containerID)
}
