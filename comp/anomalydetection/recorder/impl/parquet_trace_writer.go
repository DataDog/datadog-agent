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

// TraceParquetWriter writes observer traces to parquet files created on each flush.
// Traces are stored as denormalized spans (one row per span) for efficient columnar queries.
// Each span row includes trace-level metadata for easy filtering without joins.
// Files are only created when there is data to write; empty files are never produced.
type TraceParquetWriter struct {
	parquetWriterBase
	typedBuilder *spanBatchBuilder
}

// NewTraceParquetWriter creates a writer for trace/span data.
func NewTraceParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*TraceParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Schema for denormalized spans - each row is one span with trace context
	schema := arrow.NewSchema(
		[]arrow.Field{
			// Trace-level fields (repeated for each span in the trace)
			{Name: "RunID", Type: arrow.BinaryTypes.String},          // Source/namespace
			{Name: "Time", Type: arrow.PrimitiveTypes.Int64},         // Trace timestamp (ms since epoch)
			{Name: "TraceIDHigh", Type: arrow.PrimitiveTypes.Uint64}, // High 64 bits of trace ID
			{Name: "TraceIDLow", Type: arrow.PrimitiveTypes.Uint64},  // Low 64 bits of trace ID
			{Name: "Env", Type: arrow.BinaryTypes.String},
			{Name: "TraceService", Type: arrow.BinaryTypes.String}, // Primary service for trace
			{Name: "Hostname", Type: arrow.BinaryTypes.String},
			{Name: "ContainerID", Type: arrow.BinaryTypes.String},
			{Name: "TraceDurationNs", Type: arrow.PrimitiveTypes.Int64},
			{Name: "Priority", Type: arrow.PrimitiveTypes.Int32},
			{Name: "TraceError", Type: arrow.FixedWidthTypes.Boolean},
			{Name: "TraceTags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
			// Span-level fields
			{Name: "SpanID", Type: arrow.PrimitiveTypes.Uint64},
			{Name: "ParentID", Type: arrow.PrimitiveTypes.Uint64},
			{Name: "SpanService", Type: arrow.BinaryTypes.String},
			{Name: "SpanName", Type: arrow.BinaryTypes.String}, // Operation name
			{Name: "Resource", Type: arrow.BinaryTypes.String}, // Resource (SQL query, HTTP route, etc.)
			{Name: "SpanType", Type: arrow.BinaryTypes.String}, // web, db, cache, etc.
			{Name: "SpanStartNs", Type: arrow.PrimitiveTypes.Int64},
			{Name: "SpanDurationNs", Type: arrow.PrimitiveTypes.Int64},
			{Name: "SpanError", Type: arrow.PrimitiveTypes.Int32},
			{Name: "SpanMeta", Type: arrow.ListOf(arrow.BinaryTypes.String)},    // String tags as "key:value"
			{Name: "SpanMetrics", Type: arrow.ListOf(arrow.BinaryTypes.String)}, // Numeric metrics as "key:value"
		},
		nil,
	)

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("TraceService", true),
		parquet.WithBloomFilterFPPFor("TraceService", 0.01),
		parquet.WithBloomFilterEnabledFor("SpanService", true),
		parquet.WithBloomFilterFPPFor("SpanService", 0.01),
		parquet.WithBloomFilterEnabledFor("SpanName", true),
		parquet.WithBloomFilterFPPFor("SpanName", 0.01),
	)

	builder := newSpanBatchBuilder(schema)
	tw := &TraceParquetWriter{
		parquetWriterBase: parquetWriterBase{
			outputDir:         outputDir,
			filePrefix:        "observer-traces",
			schema:            schema,
			writerProps:       props,
			builder:           builder,
			flushInterval:     flushInterval,
			retentionDuration: retentionDuration,
			stopCh:            make(chan struct{}),
		},
		typedBuilder: builder,
	}
	tw.start()

	pkglog.Infof("Trace parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return tw, nil
}

// WriteSpan adds a span to the batch with its trace context.
func (tw *TraceParquetWriter) WriteSpan(
	source string,
	traceIDHigh, traceIDLow uint64,
	env, traceService, hostname, containerID string,
	traceTimestampNs, traceDurationNs int64,
	priority int32,
	traceError bool,
	traceTags []string,
	spanID, parentID uint64,
	spanService, spanName, resource, spanType string,
	spanStartNs, spanDurationNs int64,
	spanError int32,
	spanMeta, spanMetrics []string,
) {
	tw.mu.Lock()
	defer tw.mu.Unlock()

	tw.typedBuilder.add(
		source,
		traceIDHigh, traceIDLow,
		env, traceService, hostname, containerID,
		traceTimestampNs, traceDurationNs,
		priority, traceError, traceTags,
		spanID, parentID,
		spanService, spanName, resource, spanType,
		spanStartNs, spanDurationNs, spanError,
		spanMeta, spanMetrics,
	)
}

// spanBatchBuilder accumulates spans into Arrow record batches.
type spanBatchBuilder struct {
	schema *arrow.Schema

	// Trace-level fields
	runIDs         []string
	times          []int64
	traceIDHighs   []uint64
	traceIDLows    []uint64
	envs           []string
	traceServices  []string
	hostnames      []string
	containerIDs   []string
	traceDurations []int64
	priorities     []int32
	traceErrors    []bool
	traceTags      [][]string

	// Span-level fields
	spanIDs       []uint64
	parentIDs     []uint64
	spanServices  []string
	spanNames     []string
	resources     []string
	spanTypes     []string
	spanStarts    []int64
	spanDurations []int64
	spanErrors    []int32
	spanMetas     [][]string
	spanMetrics   [][]string
}

func newSpanBatchBuilder(schema *arrow.Schema) *spanBatchBuilder {
	return &spanBatchBuilder{schema: schema}
}

func (b *spanBatchBuilder) add(
	source string,
	traceIDHigh, traceIDLow uint64,
	env, traceService, hostname, containerID string,
	traceTimestampNs, traceDurationNs int64,
	priority int32,
	traceError bool,
	traceTags []string,
	spanID, parentID uint64,
	spanService, spanName, resource, spanType string,
	spanStartNs, spanDurationNs int64,
	spanError int32,
	spanMeta, spanMetrics []string,
) {
	b.runIDs = append(b.runIDs, source)
	b.times = append(b.times, traceTimestampNs/1000000) // Convert ns to ms
	b.traceIDHighs = append(b.traceIDHighs, traceIDHigh)
	b.traceIDLows = append(b.traceIDLows, traceIDLow)
	b.envs = append(b.envs, env)
	b.traceServices = append(b.traceServices, traceService)
	b.hostnames = append(b.hostnames, hostname)
	b.containerIDs = append(b.containerIDs, containerID)
	b.traceDurations = append(b.traceDurations, traceDurationNs)
	b.priorities = append(b.priorities, priority)
	b.traceErrors = append(b.traceErrors, traceError)

	tagsCopy := make([]string, len(traceTags))
	copy(tagsCopy, traceTags)
	b.traceTags = append(b.traceTags, tagsCopy)

	b.spanIDs = append(b.spanIDs, spanID)
	b.parentIDs = append(b.parentIDs, parentID)
	b.spanServices = append(b.spanServices, spanService)
	b.spanNames = append(b.spanNames, spanName)
	b.resources = append(b.resources, resource)
	b.spanTypes = append(b.spanTypes, spanType)
	b.spanStarts = append(b.spanStarts, spanStartNs)
	b.spanDurations = append(b.spanDurations, spanDurationNs)
	b.spanErrors = append(b.spanErrors, spanError)

	metaCopy := make([]string, len(spanMeta))
	copy(metaCopy, spanMeta)
	b.spanMetas = append(b.spanMetas, metaCopy)

	metricsCopy := make([]string, len(spanMetrics))
	copy(metricsCopy, spanMetrics)
	b.spanMetrics = append(b.spanMetrics, metricsCopy)
}

func (b *spanBatchBuilder) build() arrow.Record {
	if len(b.runIDs) == 0 {
		return nil
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	// Trace-level fields
	runIDBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	timeBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	traceIDHighBuilder := recordBuilder.Field(2).(*array.Uint64Builder)
	traceIDLowBuilder := recordBuilder.Field(3).(*array.Uint64Builder)
	envBuilder := recordBuilder.Field(4).(*array.StringBuilder)
	traceServiceBuilder := recordBuilder.Field(5).(*array.StringBuilder)
	hostnameBuilder := recordBuilder.Field(6).(*array.StringBuilder)
	containerIDBuilder := recordBuilder.Field(7).(*array.StringBuilder)
	traceDurationBuilder := recordBuilder.Field(8).(*array.Int64Builder)
	priorityBuilder := recordBuilder.Field(9).(*array.Int32Builder)
	traceErrorBuilder := recordBuilder.Field(10).(*array.BooleanBuilder)
	traceTagsBuilder := recordBuilder.Field(11).(*array.ListBuilder)
	traceTagsValueBuilder := traceTagsBuilder.ValueBuilder().(*array.StringBuilder)

	// Span-level fields
	spanIDBuilder := recordBuilder.Field(12).(*array.Uint64Builder)
	parentIDBuilder := recordBuilder.Field(13).(*array.Uint64Builder)
	spanServiceBuilder := recordBuilder.Field(14).(*array.StringBuilder)
	spanNameBuilder := recordBuilder.Field(15).(*array.StringBuilder)
	resourceBuilder := recordBuilder.Field(16).(*array.StringBuilder)
	spanTypeBuilder := recordBuilder.Field(17).(*array.StringBuilder)
	spanStartBuilder := recordBuilder.Field(18).(*array.Int64Builder)
	spanDurationBuilder := recordBuilder.Field(19).(*array.Int64Builder)
	spanErrorBuilder := recordBuilder.Field(20).(*array.Int32Builder)
	spanMetaBuilder := recordBuilder.Field(21).(*array.ListBuilder)
	spanMetaValueBuilder := spanMetaBuilder.ValueBuilder().(*array.StringBuilder)
	spanMetricsBuilder := recordBuilder.Field(22).(*array.ListBuilder)
	spanMetricsValueBuilder := spanMetricsBuilder.ValueBuilder().(*array.StringBuilder)

	for _, v := range b.runIDs {
		runIDBuilder.Append(v)
	}
	timeBuilder.AppendValues(b.times, nil)
	traceIDHighBuilder.AppendValues(b.traceIDHighs, nil)
	traceIDLowBuilder.AppendValues(b.traceIDLows, nil)
	for _, v := range b.envs {
		envBuilder.Append(v)
	}
	for _, v := range b.traceServices {
		traceServiceBuilder.Append(v)
	}
	for _, v := range b.hostnames {
		hostnameBuilder.Append(v)
	}
	for _, v := range b.containerIDs {
		containerIDBuilder.Append(v)
	}
	traceDurationBuilder.AppendValues(b.traceDurations, nil)
	priorityBuilder.AppendValues(b.priorities, nil)
	for _, v := range b.traceErrors {
		traceErrorBuilder.Append(v)
	}
	for _, tagList := range b.traceTags {
		traceTagsBuilder.Append(true)
		for _, tag := range tagList {
			traceTagsValueBuilder.Append(tag)
		}
	}

	spanIDBuilder.AppendValues(b.spanIDs, nil)
	parentIDBuilder.AppendValues(b.parentIDs, nil)
	for _, v := range b.spanServices {
		spanServiceBuilder.Append(v)
	}
	for _, v := range b.spanNames {
		spanNameBuilder.Append(v)
	}
	for _, v := range b.resources {
		resourceBuilder.Append(v)
	}
	for _, v := range b.spanTypes {
		spanTypeBuilder.Append(v)
	}
	spanStartBuilder.AppendValues(b.spanStarts, nil)
	spanDurationBuilder.AppendValues(b.spanDurations, nil)
	spanErrorBuilder.AppendValues(b.spanErrors, nil)
	for _, metaList := range b.spanMetas {
		spanMetaBuilder.Append(true)
		for _, meta := range metaList {
			spanMetaValueBuilder.Append(meta)
		}
	}
	for _, metricsList := range b.spanMetrics {
		spanMetricsBuilder.Append(true)
		for _, metric := range metricsList {
			spanMetricsValueBuilder.Append(metric)
		}
	}

	record := recordBuilder.NewRecord()
	recordBuilder.Release()

	// Reset builder
	b.runIDs = nil
	b.times = nil
	b.traceIDHighs = nil
	b.traceIDLows = nil
	b.envs = nil
	b.traceServices = nil
	b.hostnames = nil
	b.containerIDs = nil
	b.traceDurations = nil
	b.priorities = nil
	b.traceErrors = nil
	b.traceTags = nil
	b.spanIDs = nil
	b.parentIDs = nil
	b.spanServices = nil
	b.spanNames = nil
	b.resources = nil
	b.spanTypes = nil
	b.spanStarts = nil
	b.spanDurations = nil
	b.spanErrors = nil
	b.spanMetas = nil
	b.spanMetrics = nil

	return record
}
