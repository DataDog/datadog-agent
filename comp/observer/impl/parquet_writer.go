// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build observer

package observerimpl

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ParquetWriter writes observer metrics to parquet files.
// Schema is compatible with FGM (Flare Graph Metrics) format for consistency.
type ParquetWriter struct {
	filePath      string
	writer        *pqarrow.FileWriter
	file          *os.File
	schema        *arrow.Schema
	builder       *metricBatchBuilder
	flushInterval time.Duration
	stopCh        chan struct{}
	mu            sync.Mutex
}

// NewParquetWriter creates a writer that periodically flushes metrics to parquet.
func NewParquetWriter(filePath string, flushInterval time.Duration) (*ParquetWriter, error) {
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

	// Create parquet file writer
	file, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("creating parquet file: %w", err)
	}

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
	)

	writer, err := pqarrow.NewFileWriter(schema, file, props, pqarrow.DefaultWriterProps())
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("creating parquet writer: %w", err)
	}

	pw := &ParquetWriter{
		filePath:      filePath,
		writer:        writer,
		file:          file,
		schema:        schema,
		builder:       newMetricBatchBuilder(schema),
		flushInterval: flushInterval,
		stopCh:        make(chan struct{}),
	}

	// Start flush loop
	go pw.flushLoop()

	pkglog.Infof("Parquet writer initialized: %s (flush interval: %v)", filePath, flushInterval)

	return pw, nil
}

// WriteMetric adds a metric to the batch (will be flushed on interval)
func (pw *ParquetWriter) WriteMetric(source, name string, value float64, tags []string, timestamp int64) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.builder.add(source, name, value, tags, timestamp)
}

// flushLoop periodically flushes metrics to parquet file
func (pw *ParquetWriter) flushLoop() {
	ticker := time.NewTicker(pw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pw.stopCh:
			pw.flush()
			return
		case <-ticker.C:
			pw.flush()
		}
	}
}

// flush writes accumulated metrics to parquet file
func (pw *ParquetWriter) flush() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	record := pw.builder.build()
	if record == nil {
		return // No metrics to flush
	}

	if err := pw.writer.Write(record); err != nil {
		pkglog.Errorf("Failed to write metrics to parquet: %v", err)
	}

	record.Release()
}

// Close flushes remaining data and closes the writer
func (pw *ParquetWriter) Close() error {
	close(pw.stopCh)

	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Final flush
	record := pw.builder.build()
	if record != nil {
		if err := pw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write final metrics to parquet: %v", err)
		}
		record.Release()
	}

	// Close writer and file
	if err := pw.writer.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}

	if err := pw.file.Close(); err != nil {
		return fmt.Errorf("closing parquet file: %w", err)
	}

	pkglog.Infof("Parquet writer closed: %s", pw.filePath)
	return nil
}

// metricBatchBuilder accumulates metrics into Arrow record batches
type metricBatchBuilder struct {
	schema *arrow.Schema
	pool   memory.Allocator

	runIDs      []string
	times       []int64
	metricNames []string
	valueFloats []float64
	tags        [][]string
}

func newMetricBatchBuilder(schema *arrow.Schema) *metricBatchBuilder {
	return &metricBatchBuilder{
		schema:      schema,
		pool:        memory.NewGoAllocator(),
		runIDs:      []string{},
		times:       []int64{},
		metricNames: []string{},
		valueFloats: []float64{},
		tags:        [][]string{},
	}
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

	// Build Arrow arrays from accumulated data
	runIDBuilder := array.NewStringBuilder(b.pool)
	defer runIDBuilder.Release()
	for _, id := range b.runIDs {
		runIDBuilder.Append(id)
	}
	runIDArray := runIDBuilder.NewArray()
	defer runIDArray.Release()

	timeBuilder := array.NewInt64Builder(b.pool)
	defer timeBuilder.Release()
	timeBuilder.AppendValues(b.times, nil)
	timeArray := timeBuilder.NewArray()
	defer timeArray.Release()

	nameBuilder := array.NewStringBuilder(b.pool)
	defer nameBuilder.Release()
	for _, name := range b.metricNames {
		nameBuilder.Append(name)
	}
	nameArray := nameBuilder.NewArray()
	defer nameArray.Release()

	valueBuilder := array.NewFloat64Builder(b.pool)
	defer valueBuilder.Release()
	valueBuilder.AppendValues(b.valueFloats, nil)
	valueArray := valueBuilder.NewArray()
	defer valueArray.Release()

	// Build tags list column
	tagsListBuilder := array.NewListBuilder(b.pool, arrow.BinaryTypes.String)
	defer tagsListBuilder.Release()
	tagsValueBuilder := tagsListBuilder.ValueBuilder().(*array.StringBuilder)

	for _, tagList := range b.tags {
		tagsListBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}
	tagsArray := tagsListBuilder.NewArray()
	defer tagsArray.Release()

	// Create record
	record := array.NewRecord(
		b.schema,
		[]arrow.Array{runIDArray, timeArray, nameArray, valueArray, tagsArray},
		int64(len(b.metricNames)),
	)

	// Reset builder for next batch
	b.runIDs = []string{}
	b.times = []int64{}
	b.metricNames = []string{}
	b.valueFloats = []float64{}
	b.tags = [][]string{}

	return record
}

// parseTagsToMap converts tag array to map for backward compatibility
// Format: "key:value" -> map["key"] = "value"
func parseTagsToMap(tags []string) map[string]string {
	tagMap := make(map[string]string)
	for _, tag := range tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			tagMap[parts[0]] = parts[1]
		} else {
			tagMap[tag] = "" // Tag without value
		}
	}
	return tagMap
}
