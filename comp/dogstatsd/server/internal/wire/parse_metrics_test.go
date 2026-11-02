// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package wire

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseMetricSample(t *testing.T, options Options, rawSample []byte) (MetricSample, error) {
	p := testParser(t, options)

	sample, err := p.ParseMetric(rawSample)
	if err == nil && len(sample.Values) > 0 {
		t.Cleanup(func() { p.float64List.Put(sample.Values) })
	}
	return sample, err
}

const epsilon = 0.00001

func TestParseGauge(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Equal(t, 666.0, sample.Value)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666:777|g"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Len(t, sample.Values, 2)
	assert.InEpsilon(t, 666.0, sample.Values[0], epsilon)
	assert.InEpsilon(t, 777.0, sample.Values[1], epsilon)
	assert.Equal(t, Gauge, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseCounter(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:21|c"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 21.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Count, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseCounterMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666:777|c"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Len(t, sample.Values, 2)
	assert.InEpsilon(t, 666.0, sample.Values[0], epsilon)
	assert.InEpsilon(t, 777.0, sample.Values[1], epsilon)
	assert.Equal(t, Count, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseCounterWithTags(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("custom_counter:1|c|#protocol:http,bench"))

	assert.NoError(t, err)

	assert.Equal(t, "custom_counter", sample.Name)
	assert.InEpsilon(t, 1.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Count, sample.Type)
	assert.Equal(t, 2, len(sample.Tags))
	assert.Equal(t, "protocol:http", sample.Tags[0])
	assert.Equal(t, "bench", sample.Tags[1])
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseHistogram(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:21|h"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 21.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Histogram, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseHistogramrMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:21:22|h"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Len(t, sample.Values, 2)
	assert.InEpsilon(t, 21.0, sample.Values[0], epsilon)
	assert.InEpsilon(t, 22.0, sample.Values[1], epsilon)
	assert.Equal(t, Histogram, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseTimer(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:21|ms"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 21.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Timing, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseTimerMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:21:22|ms"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Len(t, sample.Values, 2)
	assert.InEpsilon(t, 21.0, sample.Values[0], epsilon)
	assert.InEpsilon(t, 22.0, sample.Values[1], epsilon)
	assert.Equal(t, Timing, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseSet(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:abc|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Equal(t, "abc", sample.SetValue)
	assert.Equal(t, Set, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseSetMultiple(t *testing.T) {
	// multiple values are not supported for set. ':' can be part of the
	// set value for backward compatibility
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:abc:def|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Equal(t, "abc:def", sample.SetValue)
	assert.Equal(t, Set, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestSampleDistribution(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:3.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 3.5, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Distribution, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.Zero(t, sample.Timestamp)
}

func TestParseDistributionMultiple(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:3.5:4.5|d"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Len(t, sample.Values, 2)
	assert.InEpsilon(t, 3.5, sample.Values[0], epsilon)
	assert.InEpsilon(t, 4.5, sample.Values[1], epsilon)
	assert.Equal(t, Distribution, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.Zero(t, sample.Timestamp)
}

func TestParseSetUnicode(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:♬†øU†øU¥ºuT0♪|s"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.Equal(t, "♬†øU†øU¥ºuT0♪", sample.SetValue)
	assert.Equal(t, Set, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeWithTags(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 2, len(sample.Tags))
	assert.Equal(t, "sometag1:somevalue1", sample.Tags[0])
	assert.Equal(t, "sometag2:somevalue2", sample.Tags[1])
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeWithNoTags(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666|g"))
	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	assert.Empty(t, sample.Tags)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeWithSampleRate(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666|g|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 0.21, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeWithPoundOnly(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("daemon:666|g|#"))

	assert.NoError(t, err)

	assert.Equal(t, "daemon", sample.Name)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	assert.Len(t, sample.Tags, 0)
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseGaugeWithUnicode(t *testing.T) {
	sample, err := parseMetricSample(t, testOptions(), []byte("♬†øU†øU¥ºuT0♪:666|g|#intitulé:T0µ"))

	assert.NoError(t, err)

	assert.Equal(t, "♬†øU†øU¥ºuT0♪", sample.Name)
	assert.InEpsilon(t, 666.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "intitulé:T0µ", sample.Tags[0])
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)
}

func TestParseMetricError(t *testing.T) {
	// not enough information
	_, err := parseMetricSample(t, testOptions(), []byte("daemon:666"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, testOptions(), []byte("daemon:666|"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, testOptions(), []byte("daemon:|g"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, testOptions(), []byte(":666|g"))
	assert.Error(t, err)

	_, err = parseMetricSample(t, testOptions(), []byte("abc666|g"))
	assert.Error(t, err)

	// unknown metadata prefix
	_, err = parseMetricSample(t, testOptions(), []byte("daemon:666|g|m:test"))
	assert.NoError(t, err)

	// invalid value
	_, err = parseMetricSample(t, testOptions(), []byte("daemon:abc|g"))
	assert.Error(t, err)

	// invalid metric type
	_, err = parseMetricSample(t, testOptions(), []byte("daemon:666|unknown"))
	assert.Error(t, err)

	// invalid sample rate
	_, err = parseMetricSample(t, testOptions(), []byte("daemon:666|g|@abc"))
	assert.Error(t, err)
}

func TestParseGaugeWithTimestamp(t *testing.T) {
	// disable the no agg pipeline

	options := testOptions()
	options.ReadTimestamps = false

	// no timestamp should be read when the no agg pipeline is off

	sample, err := parseMetricSample(t, options, []byte("metric:1234|g|#onetag|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "onetag", sample.Tags[0])
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Zero(t, sample.Timestamp)

	// re-enable the no aggregation pipeline

	options.ReadTimestamps = true

	// with tags and timestamp

	sample, err = parseMetricSample(t, options, []byte("metric:1234|g|#onetag|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "onetag", sample.Tags[0])
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100430, 0))

	// with weird tags field and timestamp

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|#|T1657100430"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 0, len(sample.Tags))
	assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100430, 0))

	// with sample rate and timestamp

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|@0.21|T1657100440"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 0, len(sample.Tags))
	assert.InEpsilon(t, 0.21, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100440, 0))

	// with tags, sample rate and timestamp

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|#thereisatag|@0.21|T1657100440"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "thereisatag", sample.Tags[0])
	assert.InEpsilon(t, 0.21, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100440, 0))

	// varying the order of the tags, sample rate and timestamp entries

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|#thereisatag|T1657100540|@0.21"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "thereisatag", sample.Tags[0])
	assert.InEpsilon(t, 0.21, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|@0.21|T1657100540|#thereisatag"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "thereisatag", sample.Tags[0])
	assert.InEpsilon(t, 0.21, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|T1657100540|@0.25|#atag"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "atag", sample.Tags[0])
	assert.InEpsilon(t, 0.25, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))

	sample, err = parseMetricSample(t, testOptions(), []byte("metric:1234|g|T1657100540|#atag|@0.25"))

	assert.NoError(t, err)

	assert.Equal(t, "metric", sample.Name)
	assert.InEpsilon(t, 1234.0, sample.Value, epsilon)
	require.Nil(t, sample.Values)
	assert.Equal(t, Gauge, sample.Type)
	require.Equal(t, 1, len(sample.Tags))
	assert.Equal(t, "atag", sample.Tags[0])
	assert.InEpsilon(t, 0.25, sample.SampleRate, epsilon)
	assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))
}

func TestParseGaugeTimestampMalformed(t *testing.T) {
	// enable the no aggregation pipeline

	options := testOptions()
	options.ReadTimestamps = true

	// bad value
	_, err := parseMetricSample(t, options, []byte("metric:1234|g|#onetag|TABCD"))
	assert.Error(t, err)

	// no value
	_, err = parseMetricSample(t, options, []byte("metric:1234|g|#onetag|T"))
	assert.Error(t, err)

	// negative value
	_, err = parseMetricSample(t, options, []byte("metric:1234|g|#onetag|T-102348932"))
	assert.Error(t, err)
}

func TestParseManyPipes(t *testing.T) {
	t.Run("Sample rate and container ID (4 pipes)", func(t *testing.T) {
		sample, err := parseMetricSample(t, testOptions(), []byte("example.metric:2.39283|d|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.Name)
		assert.InEpsilon(t, 2.39283, sample.Value, epsilon)
		require.Nil(t, sample.Values)
		assert.Equal(t, Distribution, sample.Type)
		require.Equal(t, 1, len(sample.Tags))
		assert.Equal(t, "environment:dev", sample.Tags[0])
		assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp (5 pipes)", func(t *testing.T) {
		options := testOptions()
		options.ReadTimestamps = true

		sample, err := parseMetricSample(t, options, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.Name)
		assert.InEpsilon(t, 2.39283, sample.Value, epsilon)
		require.Nil(t, sample.Values)
		assert.Equal(t, Distribution, sample.Type)
		require.Equal(t, 1, len(sample.Tags))
		assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.Tags[0])
		assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp and future extension (6 pipes)", func(t *testing.T) {

		options := testOptions()
		options.ReadTimestamps = true

		sample, err := parseMetricSample(t, options, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.Name)
		assert.InEpsilon(t, 2.39283, sample.Value, epsilon)
		require.Nil(t, sample.Values)
		assert.Equal(t, Distribution, sample.Type)
		require.Equal(t, 1, len(sample.Tags))
		assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.Tags[0])
		assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	})

	t.Run("Sample rate and container ID and timestamp and 2 future extensions (7 pipes)", func(t *testing.T) {
		options := testOptions()
		options.ReadTimestamps = true

		sample, err := parseMetricSample(t, options, []byte("example.metric:2.39283|d|T1657100540|@1.000000|#environment:dev|c:2a25f7fc8fbf573d62053d7263dd2d440c07b6ab4d2b107e50b0d4df1f2ee15f|f:wowthisisacoolfeature|f2:omgthisoneiscooler"))

		require.NoError(t, err)

		assert.Equal(t, "example.metric", sample.Name)
		assert.InEpsilon(t, 2.39283, sample.Value, epsilon)
		require.Nil(t, sample.Values)
		assert.Equal(t, Distribution, sample.Type)
		require.Equal(t, 1, len(sample.Tags))
		assert.Equal(t, sample.Timestamp, time.Unix(1657100540, 0))
		assert.Equal(t, "environment:dev", sample.Tags[0])
		assert.InEpsilon(t, 1.0, sample.SampleRate, epsilon)
	})
}

func TestParseContainerID(t *testing.T) {
	options := testOptions()
	options.OriginDetection = true

	// Testing with a container ID
	sample, err := parseMetricSample(t, options, []byte("metric:1234|g|c:1234567890abcdef"))
	require.NoError(t, err)
	assert.Equal(t, "1234567890abcdef", sample.LocalData.ContainerID)
}
