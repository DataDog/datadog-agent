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

// LogParquetWriter writes observer logs to rotating parquet files.
type LogParquetWriter struct {
	outputDir         string
	currentFilePath   string
	writer            *pqarrow.FileWriter
	file              *os.File
	schema            *arrow.Schema
	builder           *logBatchBuilder
	flushInterval     time.Duration
	retentionDuration time.Duration
	stopCh            chan struct{}
	closed            bool
	mu                sync.Mutex
}

// NewLogParquetWriter creates a writer for log data.
func NewLogParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*LogParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Schema for logs
	schema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "RunID", Type: arrow.BinaryTypes.String},
			{Name: "Time", Type: arrow.PrimitiveTypes.Int64},
			{Name: "Content", Type: arrow.BinaryTypes.Binary},
			{Name: "Status", Type: arrow.BinaryTypes.String},
			{Name: "Hostname", Type: arrow.BinaryTypes.String},
			{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
		},
		nil,
	)

	lw := &LogParquetWriter{
		outputDir:         outputDir,
		schema:            schema,
		builder:           newLogBatchBuilder(schema),
		flushInterval:     flushInterval,
		retentionDuration: retentionDuration,
		stopCh:            make(chan struct{}),
	}

	if err := lw.rotateFile(); err != nil {
		return nil, fmt.Errorf("creating initial parquet file: %w", err)
	}

	go lw.flushLoop()
	if retentionDuration > 0 {
		go lw.cleanupLoop()
	}

	pkglog.Infof("Log parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)

	return lw, nil
}

func (lw *LogParquetWriter) rotateFile() error {
	if lw.writer != nil {
		if err := lw.writer.Close(); err != nil {
			pkglog.Warnf("Error closing log parquet writer during rotation: %v", err)
		}
		lw.writer = nil
	}
	lw.file = nil

	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("observer-logs-%sZ.parquet", timestamp)
	lw.currentFilePath = filepath.Join(lw.outputDir, filename)

	file, err := os.Create(lw.currentFilePath)
	if err != nil {
		return fmt.Errorf("creating parquet file %s: %w", lw.currentFilePath, err)
	}

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Status", true),
		parquet.WithBloomFilterFPPFor("Status", 0.01),
	)

	arrowProps := pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema())

	writer, err := pqarrow.NewFileWriter(lw.schema, file, props, arrowProps)
	if err != nil {
		file.Close()
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	lw.file = file
	lw.writer = writer

	pkglog.Debugf("Rotated to new log parquet file: %s", lw.currentFilePath)

	return nil
}

// WriteLog writes log data to the parquet batch.
func (lw *LogParquetWriter) WriteLog(
	source string,
	content []byte,
	status, hostname string,
	tags []string,
	timestampMs int64,
) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	lw.builder.add(source, timestampMs, content, status, hostname, tags)
}

func (lw *LogParquetWriter) flushLoop() {
	ticker := time.NewTicker(lw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lw.stopCh:
			lw.flushAndRotate()
			return
		case <-ticker.C:
			lw.flushAndRotate()
		}
	}
}

func (lw *LogParquetWriter) flushAndRotate() {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.closed {
		return
	}

	record := lw.builder.build()
	if record != nil {
		if err := lw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write logs to parquet: %v", err)
		}
		record.Release()
	}

	if err := lw.rotateFile(); err != nil {
		pkglog.Errorf("Failed to rotate log parquet file: %v", err)
	}
}

func (lw *LogParquetWriter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-lw.stopCh:
			return
		case <-ticker.C:
			lw.cleanup()
		}
	}
}

func (lw *LogParquetWriter) cleanup() {
	entries, err := os.ReadDir(lw.outputDir)
	if err != nil {
		pkglog.Warnf("Failed to read log parquet output directory for cleanup: %v", err)
		return
	}

	cutoff := time.Now().Add(-lw.retentionDuration)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "observer-logs-") || !strings.HasSuffix(entry.Name(), ".parquet") {
			continue
		}

		filePath := filepath.Join(lw.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				pkglog.Warnf("Failed to remove old log parquet file %s: %v", filePath, err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		pkglog.Infof("Cleaned up %d old log parquet file(s)", removed)
	}
}

// Close flushes remaining data and closes the writer.
func (lw *LogParquetWriter) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.closed {
		return nil
	}
	lw.closed = true

	close(lw.stopCh)

	record := lw.builder.build()
	if record != nil {
		if err := lw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write final logs to parquet: %v", err)
		}
		record.Release()
	}

	if lw.writer != nil {
		if err := lw.writer.Close(); err != nil {
			return fmt.Errorf("closing log parquet writer: %w", err)
		}
		lw.writer = nil
	}
	lw.file = nil

	pkglog.Infof("Log parquet writer closed: %s", lw.currentFilePath)
	return nil
}

// logBatchBuilder accumulates log data into Arrow record batches.
type logBatchBuilder struct {
	schema *arrow.Schema

	runIDs    []string
	times     []int64
	contents  [][]byte
	statuses  []string
	hostnames []string
	tags      [][]string
}

func newLogBatchBuilder(schema *arrow.Schema) *logBatchBuilder {
	return &logBatchBuilder{schema: schema}
}

func (b *logBatchBuilder) add(
	source string,
	timeMs int64,
	content []byte,
	status, hostname string,
	tags []string,
) {
	b.runIDs = append(b.runIDs, source)
	b.times = append(b.times, timeMs)

	// Copy content to avoid mutation
	contentCopy := make([]byte, len(content))
	copy(contentCopy, content)
	b.contents = append(b.contents, contentCopy)

	b.statuses = append(b.statuses, status)
	b.hostnames = append(b.hostnames, hostname)

	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)
	b.tags = append(b.tags, tagsCopy)
}

func (b *logBatchBuilder) build() arrow.Record {
	if len(b.runIDs) == 0 {
		return nil
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	runIDBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	timeBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	contentBuilder := recordBuilder.Field(2).(*array.BinaryBuilder)
	statusBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	hostnameBuilder := recordBuilder.Field(4).(*array.StringBuilder)
	tagsBuilder := recordBuilder.Field(5).(*array.ListBuilder)
	tagsValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)

	for _, v := range b.runIDs {
		runIDBuilder.Append(v)
	}
	timeBuilder.AppendValues(b.times, nil)
	for _, v := range b.contents {
		contentBuilder.Append(v)
	}
	for _, v := range b.statuses {
		statusBuilder.Append(v)
	}
	for _, v := range b.hostnames {
		hostnameBuilder.Append(v)
	}
	for _, tagList := range b.tags {
		tagsBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}

	record := recordBuilder.NewRecord()
	recordBuilder.Release()

	// Reset builder
	b.runIDs = nil
	b.times = nil
	b.contents = nil
	b.statuses = nil
	b.hostnames = nil
	b.tags = nil

	return record
}
