// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ParquetWriter writes observer metrics to rotating parquet files.
// Files are rotated at the flush interval to ensure they remain valid and readable.
// Schema is compatible with FGM (Flare Graph Metrics) format for consistency.
type ParquetWriter struct {
	outputDir         string
	currentFilePath   string
	writer            *pqarrow.FileWriter
	file              *os.File
	schema            *arrow.Schema
	builder           *metricBatchBuilder
	flushInterval     time.Duration
	retentionDuration time.Duration // 0 means no cleanup
	stopCh            chan struct{}
	closed            bool
	mu                sync.Mutex
}

// NewParquetWriter creates a writer that rotates parquet files at the flush interval.
// outputDir: directory where parquet files will be written
// flushInterval: how often to rotate files (e.g., 60s creates a new file every minute)
// retentionDuration: how long to keep old files (0 = no cleanup)
func NewParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*ParquetWriter, error) {
	// Create output directory if it doesn't exist
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

	pw := &ParquetWriter{
		outputDir:         outputDir,
		schema:            schema,
		builder:           newMetricBatchBuilder(schema),
		flushInterval:     flushInterval,
		retentionDuration: retentionDuration,
		stopCh:            make(chan struct{}),
	}

	// Create initial file
	if err := pw.rotateFile(); err != nil {
		return nil, fmt.Errorf("creating initial parquet file: %w", err)
	}

	// Start flush and cleanup loops
	go pw.flushLoop()
	if retentionDuration > 0 {
		go pw.cleanupLoop()
	}

	pkglog.Infof("Parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)

	return pw, nil
}

// rotateFile closes the current file and opens a new timestamped one
func (pw *ParquetWriter) rotateFile() error {
	// Close existing writer (this also closes the underlying file)
	if pw.writer != nil {
		if err := pw.writer.Close(); err != nil {
			pkglog.Warnf("Error closing parquet writer during rotation: %v", err)
		}
		pw.writer = nil
	}
	// File is already closed by writer.Close(), just nil it out
	pw.file = nil

	// Generate timestamped filename with UTC timezone: observer-metrics-20260129-133045Z.parquet
	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("observer-metrics-%sZ.parquet", timestamp)
	pw.currentFilePath = filepath.Join(pw.outputDir, filename)

	// Create new file
	file, err := os.Create(pw.currentFilePath)
	if err != nil {
		return fmt.Errorf("creating parquet file %s: %w", pw.currentFilePath, err)
	}

	// Configure parquet writer with compression and bloom filters
	// Bloom filters enable fast tag queries without reading all data
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

	// WithStoreSchema embeds the Arrow schema into Parquet metadata,
	// enabling proper reconstruction of nested types like list<string>
	arrowProps := pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema())

	writer, err := pqarrow.NewFileWriter(pw.schema, file, props, arrowProps)
	if err != nil {
		file.Close()
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	pw.file = file
	pw.writer = writer

	pkglog.Debugf("Rotated to new parquet file: %s", pw.currentFilePath)

	return nil
}

// WriteMetric adds a metric to the batch (will be flushed on interval)
func (pw *ParquetWriter) WriteMetric(source, name string, value float64, tags []string, timestamp int64) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.builder.add(source, name, value, tags, timestamp)
}

// flushLoop periodically flushes metrics and rotates files
func (pw *ParquetWriter) flushLoop() {
	ticker := time.NewTicker(pw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pw.stopCh:
			pw.flushAndRotate()
			return
		case <-ticker.C:
			pw.flushAndRotate()
		}
	}
}

// flushAndRotate writes accumulated metrics, closes file, and opens a new one
func (pw *ParquetWriter) flushAndRotate() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Don't rotate if we're closing - Close() will handle final flush
	if pw.closed {
		return
	}

	// Write accumulated metrics to current file
	record := pw.builder.build()
	if record != nil {
		if err := pw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write metrics to parquet: %v", err)
		}
		record.Release()
	}

	// Rotate to new file (closes current file, making it valid and readable)
	if err := pw.rotateFile(); err != nil {
		pkglog.Errorf("Failed to rotate parquet file: %v", err)
	}
}

// cleanupLoop periodically removes old parquet files beyond retention period
func (pw *ParquetWriter) cleanupLoop() {
	// Run cleanup every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-pw.stopCh:
			return
		case <-ticker.C:
			pw.cleanup()
		}
	}
}

// cleanup removes parquet files older than retention duration
func (pw *ParquetWriter) cleanup() {
	entries, err := os.ReadDir(pw.outputDir)
	if err != nil {
		pkglog.Warnf("Failed to read parquet output directory for cleanup: %v", err)
		return
	}

	cutoff := time.Now().Add(-pw.retentionDuration)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".parquet") {
			continue
		}

		filePath := filepath.Join(pw.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			pkglog.Warnf("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				pkglog.Warnf("Failed to remove old parquet file %s: %v", filePath, err)
			} else {
				removed++
				pkglog.Debugf("Removed old parquet file: %s", filePath)
			}
		}
	}

	if removed > 0 {
		pkglog.Infof("Cleaned up %d old parquet file(s)", removed)
	}
}

// Close flushes remaining data and closes the writer
func (pw *ParquetWriter) Close() error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	// Check if already closed
	if pw.closed {
		return nil
	}
	pw.closed = true

	// Signal background goroutines to stop
	close(pw.stopCh)

	// Final flush
	record := pw.builder.build()
	if record != nil {
		if err := pw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write final metrics to parquet: %v", err)
		}
		record.Release()
	}

	// Close writer (this also closes the underlying file)
	if pw.writer != nil {
		if err := pw.writer.Close(); err != nil {
			return fmt.Errorf("closing parquet writer: %w", err)
		}
		pw.writer = nil
	}
	// File is already closed by writer.Close()
	pw.file = nil

	pkglog.Infof("Parquet writer closed: %s", pw.currentFilePath)
	return nil
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
	return &metricBatchBuilder{
		schema:      schema,
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

	// Use RecordBuilder for proper nested type handling (list<string> Tags)
	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	runIDBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	timeBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	nameBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	valueBuilder := recordBuilder.Field(3).(*array.Float64Builder)
	tagsBuilder := recordBuilder.Field(4).(*array.ListBuilder)
	tagsValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)

	// Append all accumulated data
	for _, id := range b.runIDs {
		runIDBuilder.Append(id)
	}
	timeBuilder.AppendValues(b.times, nil)
	for _, name := range b.metricNames {
		nameBuilder.Append(name)
	}
	valueBuilder.AppendValues(b.valueFloats, nil)

	// Append tags for each metric
	for _, tagList := range b.tags {
		tagsBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}

	// Create record
	record := recordBuilder.NewRecord()
	recordBuilder.Release()

	// Reset builder for next batch
	b.runIDs = []string{}
	b.times = []int64{}
	b.metricNames = []string{}
	b.valueFloats = []float64{}
	b.tags = [][]string{}

	return record
}

