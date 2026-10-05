// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/stretchr/testify/require"

	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

func readOneBatch(t *testing.T, pattern string, bloomColumns []string, check func(arrow.RecordBatch)) {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	require.NoError(t, err)
	require.Len(t, paths, 1)
	f, err := os.Open(paths[0])
	require.NoError(t, err)
	defer f.Close()
	pf, err := file.NewParquetReader(f)
	require.NoError(t, err)
	defer pf.Close()
	meta := pf.MetaData()
	require.NotNil(t, meta.KeyValueMetadata().FindValue("ARROW:schema"))
	require.Equal(t, 1, meta.NumRowGroups())
	for i := 0; i < meta.RowGroup(0).NumColumns(); i++ {
		column, err := meta.RowGroup(0).ColumnChunk(i)
		require.NoError(t, err)
		require.Equal(t, compress.Codecs.Zstd, column.Compression())
	}
	if len(bloomColumns) > 0 {
		assertBloomFilters(t, pf, bloomColumns)
	}
	r, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
	require.NoError(t, err)
	rr, err := r.GetRecordReader(context.Background(), nil, nil)
	require.NoError(t, err)
	defer rr.Release()
	require.True(t, rr.Next())
	check(rr.Record())
	require.False(t, rr.Next())
}

func TestMetricParquetV1Format(t *testing.T) {
	dir := t.TempDir()
	w, err := (Factory{}).NewMetricWriter(recorder.WriterConfig{OutputDir: dir, FlushInterval: time.Hour})
	require.NoError(t, err)
	require.True(t, w.WriteMetric(recorder.MetricData{
		Source: "check", Name: "system.cpu", Value: 2.5, Timestamp: 1234,
		Tags: []string{"host:test", "env:dev"}, Dropped: true, MetricType: "Gauge",
	}))
	require.True(t, w.WriteMetric(recorder.MetricData{
		Source: "dogstatsd", Name: "request.count", Value: 7, Timestamp: 1235,
		Tags: []string{"service:api"}, Dropped: false, MetricType: "Count",
	}))
	require.True(t, w.WriteMetric(recorder.MetricData{Source: "check", Name: "legacy", Timestamp: 1236}))
	require.NoError(t, w.Close())

	readOneBatch(t, filepath.Join(dir, "observer-metrics-*.parquet"), []string{"MetricName", "Tags.list.element"}, func(rec arrow.RecordBatch) {
		require.Equal(t, []string{"RunID", "Time", "MetricName", "ValueFloat", "Tags", "Dropped", "MetricType"}, fieldNames(rec.Schema()))
		require.Equal(t, []string{"utf8", "int64", "utf8", "float64", "list<element: utf8, nullable>", "bool", "utf8"}, fieldTypes(rec.Schema()))
		require.Equal(t, int64(3), rec.NumRows())
		require.Equal(t, "check", rec.Column(0).(*array.String).Value(0))
		require.Equal(t, int64(1234000), rec.Column(1).(*array.Int64).Value(0))
		require.Equal(t, "system.cpu", rec.Column(2).(*array.String).Value(0))
		require.Equal(t, 2.5, rec.Column(3).(*array.Float64).Value(0))
		require.Equal(t, []string{"host:test", "env:dev"}, listValues(rec.Column(4).(*array.List), 0))
		require.True(t, rec.Column(5).(*array.Boolean).Value(0))
		require.Equal(t, "Gauge", rec.Column(6).(*array.String).Value(0))
		require.Equal(t, "dogstatsd", rec.Column(0).(*array.String).Value(1))
		require.Equal(t, int64(1235000), rec.Column(1).(*array.Int64).Value(1))
		require.Equal(t, []string{"service:api"}, listValues(rec.Column(4).(*array.List), 1))
		require.False(t, rec.Column(5).(*array.Boolean).Value(1))
		require.Equal(t, "Count", rec.Column(6).(*array.String).Value(1))
		require.Equal(t, "Unknown", rec.Column(6).(*array.String).Value(2))
	})
}

func TestLogParquetV1Format(t *testing.T) {
	dir := t.TempDir()
	w, err := (Factory{}).NewLogWriter(recorder.WriterConfig{OutputDir: dir, FlushInterval: time.Hour})
	require.NoError(t, err)
	require.True(t, w.WriteLog(recorder.LogData{
		Source: "logs", TimestampMs: 1234567, Content: []byte{0, 'x', 255},
		Status: "error", Hostname: "test-host", Tags: []string{"service:api"},
	}))
	require.NoError(t, w.Close())

	readOneBatch(t, filepath.Join(dir, "observer-logs-*.parquet"), []string{"Status"}, func(rec arrow.RecordBatch) {
		require.Equal(t, []string{"RunID", "Time", "Content", "Status", "Hostname", "Tags"}, fieldNames(rec.Schema()))
		require.Equal(t, []string{"utf8", "int64", "binary", "utf8", "utf8", "list<element: utf8, nullable>"}, fieldTypes(rec.Schema()))
		require.Equal(t, int64(1), rec.NumRows())
		require.Equal(t, "logs", rec.Column(0).(*array.String).Value(0))
		require.Equal(t, int64(1234567), rec.Column(1).(*array.Int64).Value(0))
		require.Equal(t, []byte{0, 'x', 255}, rec.Column(2).(*array.Binary).Value(0))
		require.Equal(t, "error", rec.Column(3).(*array.String).Value(0))
		require.Equal(t, "test-host", rec.Column(4).(*array.String).Value(0))
		require.Equal(t, []string{"service:api"}, listValues(rec.Column(5).(*array.List), 0))
	})
}

func assertBloomFilters(t *testing.T, reader *file.Reader, paths []string) {
	t.Helper()
	rowGroup, err := reader.GetBloomFilterReader().RowGroup(0)
	require.NoError(t, err)
	columns := reader.MetaData().RowGroup(0)
	for _, path := range paths {
		found := false
		for i := 0; i < columns.NumColumns(); i++ {
			column, err := columns.ColumnChunk(i)
			require.NoError(t, err)
			if column.PathInSchema().String() != path {
				continue
			}
			found = true
			require.Positive(t, column.BloomFilterOffset(), path)
			require.Positive(t, column.BloomFilterLength(), path)
			filter, err := rowGroup.GetColumnBloomFilter(i)
			require.NoError(t, err, path)
			require.NotNil(t, filter, path)
		}
		require.True(t, found, "missing parquet column %s", path)
	}
}

func fieldNames(schema *arrow.Schema) []string {
	var names []string
	for _, field := range schema.Fields() {
		names = append(names, field.Name)
	}
	return names
}

func fieldTypes(schema *arrow.Schema) []string {
	var types []string
	for _, field := range schema.Fields() {
		types = append(types, field.Type.String())
	}
	return types
}

func listValues(list *array.List, row int) []string {
	start, end := list.ValueOffsets(row)
	values := list.ListValues().(*array.String)
	result := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		result = append(result, values.Value(int(i)))
	}
	return result
}
