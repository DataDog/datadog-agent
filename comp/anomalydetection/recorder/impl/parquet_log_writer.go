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

// LogParquetWriter writes observer logs to parquet files created on each flush.
// Files are only created when there is data to write; empty files are never produced.
type LogParquetWriter struct {
	parquetWriterBase
	typedBuilder *logBatchBuilder
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

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Status", true),
		parquet.WithBloomFilterFPPFor("Status", 0.01),
	)

	builder := newLogBatchBuilder(schema)
	lw := &LogParquetWriter{
		parquetWriterBase: parquetWriterBase{
			outputDir:         outputDir,
			filePrefix:        "observer-logs",
			schema:            schema,
			writerProps:       props,
			builder:           builder,
			flushInterval:     flushInterval,
			retentionDuration: retentionDuration,
			stopCh:            make(chan struct{}),
		},
		typedBuilder: builder,
	}
	lw.start()

	pkglog.Infof("Log parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return lw, nil
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

	lw.typedBuilder.add(source, timestampMs, content, status, hostname, tags)
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
