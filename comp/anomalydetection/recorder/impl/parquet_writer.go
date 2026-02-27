// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"fmt"
	"os"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ParquetWriter writes observer metrics to parquet files created on each flush.
// Files are only created when there is data to write; empty files are never produced.
// Schema is compatible with FGM (Flare Graph Metrics) format for consistency.
type ParquetWriter struct {
	parquetWriterBase
	typedBuilder *metricBatchBuilder
}

// NewParquetWriter creates a writer that rotates parquet files at the flush interval.
// outputDir: directory where parquet files will be written
// flushInterval: how often to rotate files (e.g., 60s creates a new file every minute)
// retentionDuration: how long to keep old files (0 = no cleanup)
func NewParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*ParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Define schema matching FGM format for compatibility with existing reader
	schema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "RunID", Type: arrow.BinaryTypes.String},              // Source/namespace
			{Name: "Time", Type: arrow.PrimitiveTypes.Int64},             // milliseconds since epoch
			{Name: "MetricName", Type: arrow.BinaryTypes.String},         // metric name
			{Name: "ValueFloat", Type: arrow.PrimitiveTypes.Float64},     // metric value
			{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)}, // tags as list of strings
		},
		nil,
	)

	// Configure parquet writer with compression and bloom filters.
	// Bloom filters enable fast tag queries without reading all data.
	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		// Enable bloom filter on Tags column for fast tag queries
		parquet.WithBloomFilterEnabledFor("Tags", true),
		parquet.WithBloomFilterFPPFor("Tags", 0.01), // 1% false positive rate
		// Also enable on MetricName for fast metric filtering
		parquet.WithBloomFilterEnabledFor("MetricName", true),
		parquet.WithBloomFilterFPPFor("MetricName", 0.01),
	)

	builder := newMetricBatchBuilder(schema)
	pw := &ParquetWriter{
		parquetWriterBase: parquetWriterBase{
			outputDir:         outputDir,
			filePrefix:        "observer-metrics",
			schema:            schema,
			writerProps:       props,
			builder:           builder,
			flushInterval:     flushInterval,
			retentionDuration: retentionDuration,
			stopCh:            make(chan struct{}),
		},
		typedBuilder: builder,
	}
	pw.start()

	pkglog.Infof("Parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return pw, nil
}

// WriteMetric adds a metric to the batch (will be flushed on interval).
func (pw *ParquetWriter) WriteMetric(source, name string, value float64, tags []string, timestamp int64) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.typedBuilder.add(source, name, value, tags, timestamp)
}

// metricBatchBuilder accumulates metrics into Arrow record batches using RecordBuilder
type metricBatchBuilder struct {
	schema *arrow.Schema

	runIDs      []string
	times       []int64
	metricNames []string
	valueFloats []float64
	tags        [][]string
}

func newMetricBatchBuilder(schema *arrow.Schema) *metricBatchBuilder {
	return &metricBatchBuilder{schema: schema}
}

func (b *metricBatchBuilder) add(source, name string, value float64, tags []string, timestamp int64) {
	b.runIDs = append(b.runIDs, source)
	b.times = append(b.times, timestamp*1000) // Convert to milliseconds
	b.metricNames = append(b.metricNames, name)
	b.valueFloats = append(b.valueFloats, value)

	// Copy tags to avoid mutation
	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)
	b.tags = append(b.tags, tagsCopy)
}

func (b *metricBatchBuilder) build() arrow.Record {
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

	for _, id := range b.runIDs {
		runIDBuilder.Append(id)
	}
	timeBuilder.AppendValues(b.times, nil)
	for _, name := range b.metricNames {
		nameBuilder.Append(name)
	}
	valueBuilder.AppendValues(b.valueFloats, nil)

	for _, tagList := range b.tags {
		tagsBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}

	record := recordBuilder.NewRecord()
	recordBuilder.Release()

	// Reset builder for next batch
	b.runIDs = nil
	b.times = nil
	b.metricNames = nil
	b.valueFloats = nil
	b.tags = nil

	return record
}
