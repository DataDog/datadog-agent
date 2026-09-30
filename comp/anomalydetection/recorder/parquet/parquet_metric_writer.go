// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"fmt"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
)

// metricParquetWriter batches metrics in the v1 schema.
type metricParquetWriter struct {
	parquetWriter
	typedBuilder *metricBatchBuilder
}

// newMetricParquetWriter starts periodic metric flushes.
func newMetricParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*metricParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Keep the v1 schema used by the testbench reader.
	schema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "RunID", Type: arrow.BinaryTypes.String},              // Source/namespace
			{Name: "Time", Type: arrow.PrimitiveTypes.Int64},             // milliseconds since epoch
			{Name: "MetricName", Type: arrow.BinaryTypes.String},         // metric name
			{Name: "ValueFloat", Type: arrow.PrimitiveTypes.Float64},     // metric value
			{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)}, // tags as list of strings
			{Name: "Dropped", Type: arrow.FixedWidthTypes.Boolean},       // true if live channel dropped this observation
		},
		nil,
	)

	// Preserve the v1 compression and bloom filters.
	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Tags", true),
		parquet.WithBloomFilterFPPFor("Tags", 0.01), // 1% false positive rate
		parquet.WithBloomFilterEnabledFor("MetricName", true),
		parquet.WithBloomFilterFPPFor("MetricName", 0.01),
	)

	builder := newMetricBatchBuilder(schema)
	pw := &metricParquetWriter{
		parquetWriter: parquetWriter{
			outputDir:         outputDir,
			filePrefix:        "observer-metrics",
			schema:            schema,
			writerProps:       props,
			builder:           builder,
			flushInterval:     flushInterval,
			retentionDuration: retentionDuration,
			stopCh:            make(chan struct{}),
			now:               time.Now,
		},
		typedBuilder: builder,
	}
	pw.start()

	logging.Infof("Parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return pw, nil
}

// WriteMetric adds a stable metric snapshot to the next batch.
func (pw *metricParquetWriter) WriteMetric(metric recorder.MetricData) bool {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.closed {
		return false
	}
	pw.typedBuilder.add(metric)
	return true
}

// metricBatchBuilder accumulates metrics into Arrow record batches using RecordBuilder
type metricBatchBuilder struct {
	schema *arrow.Schema

	runIDs      []string
	times       []int64
	metricNames []string
	valueFloats []float64
	tags        [][]string
	dropped     []bool
}

func newMetricBatchBuilder(schema *arrow.Schema) *metricBatchBuilder {
	return &metricBatchBuilder{schema: schema}
}

func (b *metricBatchBuilder) add(metric recorder.MetricData) {
	b.runIDs = append(b.runIDs, metric.Source)
	b.times = append(b.times, metric.Timestamp*1000)
	b.metricNames = append(b.metricNames, metric.Name)
	b.valueFloats = append(b.valueFloats, metric.Value)
	b.dropped = append(b.dropped, metric.Dropped)
	b.tags = append(b.tags, metric.Tags)
}

func (b *metricBatchBuilder) build() arrow.RecordBatch {
	if len(b.metricNames) == 0 {
		return nil
	}

	// Use RecordBuilder for proper nested type handling (list<string> Tags)
	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	runIDBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	timeBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	nameBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	valueBuilder := recordBuilder.Field(3).(*array.Float64Builder)
	tagsBuilder := recordBuilder.Field(4).(*array.ListBuilder)
	tagsValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)
	droppedBuilder := recordBuilder.Field(5).(*array.BooleanBuilder)

	for _, id := range b.runIDs {
		runIDBuilder.Append(id)
	}
	timeBuilder.AppendValues(b.times, nil)
	for _, name := range b.metricNames {
		nameBuilder.Append(name)
	}
	valueBuilder.AppendValues(b.valueFloats, nil)
	droppedBuilder.AppendValues(b.dropped, nil)

	for _, tagList := range b.tags {
		tagsBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}

	record := recordBuilder.NewRecordBatch()
	recordBuilder.Release()

	// Reset builder for next batch
	b.runIDs = nil
	b.times = nil
	b.metricNames = nil
	b.valueFloats = nil
	b.tags = nil
	b.dropped = nil

	return record
}
