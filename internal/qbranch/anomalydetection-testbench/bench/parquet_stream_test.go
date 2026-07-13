// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/stretchr/testify/require"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

var benchmarkLogSink []recorderdef.LogData

func TestStreamOrderedLogsV1MatchesBatchReader(t *testing.T) {
	dir := t.TempDir()
	want := []recorderdef.LogData{
		{Source: "run", TimestampMs: 1000, Content: []byte("first"), Status: "info", Hostname: "host-a", Tags: []string{"service:api"}},
		{Source: "run", TimestampMs: 1000, Content: []byte("same-time"), Status: "warn", Hostname: "host-a", Tags: []string{"service:api", "env:test"}},
		{Source: "run", TimestampMs: 1001, Content: []byte("last"), Status: "error", Hostname: "host-b"},
	}
	writeLogParquetV1(t, filepath.Join(dir, "observer-logs-000000.parquet"), want[:2], 1)
	writeLogParquetV1(t, filepath.Join(dir, "observer-logs-000001.parquet"), want[2:], 1)

	var got []recorderdef.LogData
	count, err := streamOrderedLogs(dir, FormatV1, func(entry recorderdef.LogData) error {
		got = append(got, entry)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, len(want), count)
	require.Equal(t, want, got)

	batch, err := readAllLogs(dir)
	require.NoError(t, err)
	require.Equal(t, batch, got)
}

func TestStreamOrderedLogsV1RejectsDisorder(t *testing.T) {
	t.Run("within a file", func(t *testing.T) {
		dir := t.TempDir()
		writeLogParquetV1(t, filepath.Join(dir, "observer-logs-000000.parquet"), []recorderdef.LogData{
			{TimestampMs: 1001},
			{TimestampMs: 1000},
		}, 1)

		count, err := streamOrderedLogs(dir, FormatV1, func(recorderdef.LogData) error { return nil })
		require.Equal(t, 1, count)
		require.ErrorContains(t, err, "observer-logs-000000.parquet contains 1000 after 1001")
	})

	t.Run("across files", func(t *testing.T) {
		dir := t.TempDir()
		writeLogParquetV1(t, filepath.Join(dir, "observer-logs-000000.parquet"), []recorderdef.LogData{{TimestampMs: 1001}}, 1)
		writeLogParquetV1(t, filepath.Join(dir, "observer-logs-000001.parquet"), []recorderdef.LogData{{TimestampMs: 1000}}, 1)

		count, err := streamOrderedLogs(dir, FormatV1, func(recorderdef.LogData) error { return nil })
		require.Equal(t, 1, count)
		require.ErrorContains(t, err, "observer-logs-000001.parquet contains 1000 after 1001")
	})
}

func TestStreamOrderedMetricsV1MatchesBatchReader(t *testing.T) {
	dir := t.TempDir()
	want := []recorderdef.MetricData{
		{Source: "run", Name: "system.cpu", Value: 1.5, Timestamp: 1000, Tags: []string{"host:a"}},
		{Source: "run", Name: "system.cpu", Value: 2.5, Timestamp: 1001, Tags: []string{"host:a"}},
		{Source: "run", Name: "system.memory", Value: 3.5, Timestamp: 1002, Tags: []string{"host:b"}},
	}
	writeMetricParquetV1(t, filepath.Join(dir, "observer-metrics-000000.parquet"), want[:2], 1)
	writeMetricParquetV1(t, filepath.Join(dir, "observer-metrics-000001.parquet"), want[2:], 1)

	var got []recorderdef.MetricData
	count, err := streamOrderedMetrics(dir, FormatV1, func(metric recorderdef.MetricData) error {
		got = append(got, metric)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, len(want), count)
	require.Equal(t, want, got)

	batch, err := readAllMetrics(dir)
	require.NoError(t, err)
	require.Equal(t, batch, got)
}

func TestStreamOrderedMetricsV1RejectsDisorderAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	writeMetricParquetV1(t, filepath.Join(dir, "observer-metrics-000000.parquet"), []recorderdef.MetricData{{Timestamp: 1001}}, 1)
	writeMetricParquetV1(t, filepath.Join(dir, "observer-metrics-000001.parquet"), []recorderdef.MetricData{{Timestamp: 1000}}, 1)

	count, err := streamOrderedMetrics(dir, FormatV1, func(recorderdef.MetricData) error { return nil })
	require.Equal(t, 1, count)
	require.ErrorContains(t, err, "observer-metrics-000001.parquet contains 1000 after 1001")
}

func BenchmarkLogParquetLoading(b *testing.B) {
	dir := b.TempDir()
	logs := make([]recorderdef.LogData, 20_000)
	for i := range logs {
		content := make([]byte, 512)
		content[0] = byte(i)
		logs[i] = recorderdef.LogData{
			Source:      "run",
			TimestampMs: int64(i),
			Content:     content,
			Status:      "info",
			Hostname:    "host-a",
			Tags:        []string{"service:api", "env:test"},
		}
	}
	writeLogParquetV1(b, filepath.Join(dir, "observer-logs-000000.parquet"), logs, 1024)

	b.Run("batch", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			b.StartTimer()
			loaded, err := readAllLogs(dir)
			b.StopTimer()
			require.NoError(b, err)
			require.Len(b, loaded, len(logs))
			benchmarkLogSink = loaded
			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(max(0, int64(after.HeapAlloc)-int64(before.HeapAlloc))), "retained-B")
			benchmarkLogSink = nil
		}
	})

	b.Run("stream", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			b.StopTimer()
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)
			b.StartTimer()
			count, err := streamOrderedLogs(dir, FormatV1, func(recorderdef.LogData) error { return nil })
			b.StopTimer()
			require.NoError(b, err)
			require.Equal(b, len(logs), count)
			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			b.ReportMetric(float64(max(0, int64(after.HeapAlloc)-int64(before.HeapAlloc))), "retained-B")
		}
	})
}

func writeLogParquetV1(t testing.TB, path string, logs []recorderdef.LogData, rowGroupSize int64) {
	t.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "RunID", Type: arrow.BinaryTypes.String},
		{Name: "Time", Type: arrow.PrimitiveTypes.Int64},
		{Name: "Content", Type: arrow.BinaryTypes.Binary, Nullable: true},
		{Name: "Status", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "Hostname", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: true},
	}, nil)

	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	runIDBuilder := builder.Field(0).(*array.StringBuilder)
	timeBuilder := builder.Field(1).(*array.Int64Builder)
	contentBuilder := builder.Field(2).(*array.BinaryBuilder)
	statusBuilder := builder.Field(3).(*array.StringBuilder)
	hostnameBuilder := builder.Field(4).(*array.StringBuilder)
	tagsBuilder := builder.Field(5).(*array.ListBuilder)
	tagValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)

	for _, entry := range logs {
		runIDBuilder.Append(entry.Source)
		timeBuilder.Append(entry.TimestampMs)
		contentBuilder.Append(entry.Content)
		statusBuilder.Append(entry.Status)
		hostnameBuilder.Append(entry.Hostname)
		if entry.Tags == nil {
			tagsBuilder.AppendNull()
		} else {
			tagsBuilder.Append(true)
			tagValueBuilder.AppendValues(entry.Tags, nil)
		}
	}

	record := builder.NewRecord()
	defer record.Release()
	table := array.NewTableFromRecords(schema, []arrow.Record{record})
	defer table.Release()

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pqarrow.WriteTable(table, f, rowGroupSize, nil, pqarrow.DefaultWriterProps()))
}

func writeMetricParquetV1(t testing.TB, path string, metrics []recorderdef.MetricData, rowGroupSize int64) {
	t.Helper()

	schema := arrow.NewSchema([]arrow.Field{
		{Name: "RunID", Type: arrow.BinaryTypes.String},
		{Name: "Time", Type: arrow.PrimitiveTypes.Int64},
		{Name: "MetricName", Type: arrow.BinaryTypes.String},
		{Name: "ValueFloat", Type: arrow.PrimitiveTypes.Float64},
		{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String), Nullable: true},
		{Name: "Dropped", Type: arrow.FixedWidthTypes.Boolean},
	}, nil)

	builder := array.NewRecordBuilder(memory.DefaultAllocator, schema)
	defer builder.Release()

	runIDBuilder := builder.Field(0).(*array.StringBuilder)
	timeBuilder := builder.Field(1).(*array.Int64Builder)
	metricNameBuilder := builder.Field(2).(*array.StringBuilder)
	valueBuilder := builder.Field(3).(*array.Float64Builder)
	tagsBuilder := builder.Field(4).(*array.ListBuilder)
	tagValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)
	droppedBuilder := builder.Field(5).(*array.BooleanBuilder)

	for _, metric := range metrics {
		runIDBuilder.Append(metric.Source)
		timeBuilder.Append(metric.Timestamp * 1000)
		metricNameBuilder.Append(metric.Name)
		valueBuilder.Append(metric.Value)
		if metric.Tags == nil {
			tagsBuilder.AppendNull()
		} else {
			tagsBuilder.Append(true)
			tagValueBuilder.AppendValues(metric.Tags, nil)
		}
		droppedBuilder.Append(metric.Dropped)
	}

	record := builder.NewRecord()
	defer record.Release()
	table := array.NewTableFromRecords(schema, []arrow.Record{record})
	defer table.Release()

	f, err := os.Create(path)
	require.NoError(t, err)
	require.NoError(t, pqarrow.WriteTable(table, f, rowGroupSize, nil, pqarrow.DefaultWriterProps()))
}
