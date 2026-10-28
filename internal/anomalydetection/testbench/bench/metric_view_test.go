// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewParquetMetricViewResolvesHostTag(t *testing.T) {
	view := newParquetMetricView("system.cpu", 1, []string{"env:prod", "host:web-1", "service:api"}, 100, "", 1)

	assert.Equal(t, "web-1", view.GetHost())
	assert.Equal(t, []string{"env:prod", "service:api"}, view.GetTags().UnsafeToReadOnlySliceString())
}

func TestMetricTypeFallbackForNullableParquetColumn(t *testing.T) {
	schema := arrow.NewSchema([]arrow.Field{{Name: "MetricType", Type: arrow.BinaryTypes.String, Nullable: true}}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()
	types := builder.Field(0).(*array.StringBuilder)
	types.AppendNull()
	types.Append("")
	types.Append("future-type")
	record := builder.NewRecord()
	defer record.Release()

	rows, err := extractMetricsFromRecord(record)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	for _, row := range rows {
		view := newParquetMetricView(row.MetricName, 0, nil, 0, row.MetricType, 1)
		assert.Equal(t, observerdef.UnknownType, view.GetMetricType())
		assert.Equal(t, float64(1), view.GetSampleRate())
	}
}

func TestSampleRateFallbackForNullableParquetColumn(t *testing.T) {
	schema := arrow.NewSchema([]arrow.Field{{Name: "SampleRate", Type: arrow.PrimitiveTypes.Float64, Nullable: true}}, nil)
	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()
	rates := builder.Field(0).(*array.Float64Builder)
	rates.AppendNull()
	rates.Append(0)
	rates.Append(0.1)
	record := builder.NewRecord()
	defer record.Release()

	rows, err := extractMetricsFromRecord(record)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	expected := []float64{1, 0, 0.1}
	for i, row := range rows {
		data := metricDataFromFGM(row)
		require.Equal(t, expected[i], data.SampleRate)
		view := newParquetMetricView(data.Name, data.Value, data.Tags, data.Timestamp, data.MetricType, data.SampleRate)
		require.Equal(t, expected[i], view.GetSampleRate())
	}
}

func TestParseMetricType(t *testing.T) {
	for metricType := metrics.GaugeType; metricType < metrics.NumMetricTypes; metricType++ {
		assert.Equal(t, metricType, parseMetricType(metricType.String()))
	}
	for _, name := range []string{"", "Unknown", "future-type"} {
		assert.Equal(t, observerdef.UnknownType, parseMetricType(name))
	}
}

func TestParseSeriesKeyIncludesHost(t *testing.T) {
	namespace, name, host, tags, ok := parseSeriesKey("parquet|system.cpu:avg|web-1|env:prod,service:api")

	assert.True(t, ok)
	assert.Equal(t, "parquet", namespace)
	assert.Equal(t, "system.cpu:avg", name)
	assert.Equal(t, "web-1", host)
	assert.Equal(t, []string{"env:prod", "service:api"}, tags)
}

func TestCompositeTagsMatchIgnoresOrderAndDuplicates(t *testing.T) {
	tags := tagset.NewCompositeTags([]string{"service:api", "env:prod"}, []string{"service:api"})

	assert.True(t, compositeTagsMatch(tags, []string{"env:prod", "service:api"}))
}
