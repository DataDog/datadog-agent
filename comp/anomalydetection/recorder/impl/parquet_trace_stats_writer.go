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

// TraceStatsParquetWriter writes APM trace stats to parquet files created on each flush.
// Stats are stored as denormalized rows (one row per ClientGroupedStats) for efficient
// columnar queries. Each row includes payload- and client-level context.
// Files are only created when there is data to write; empty files are never produced.
type TraceStatsParquetWriter struct {
	parquetWriterBase
	typedBuilder *traceStatsBatchBuilder
}

// NewTraceStatsParquetWriter creates a writer for APM trace stats.
func NewTraceStatsParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*TraceStatsParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Schema: one row per ClientGroupedStats, denormalized with payload/client/bucket context.
	schema := arrow.NewSchema(
		[]arrow.Field{
			// Source
			{Name: "RunID", Type: arrow.BinaryTypes.String},
			// Agent-level (from StatsPayload)
			{Name: "AgentHostname", Type: arrow.BinaryTypes.String},
			{Name: "AgentEnv", Type: arrow.BinaryTypes.String},
			// Client-level (from ClientStatsPayload)
			{Name: "ClientHostname", Type: arrow.BinaryTypes.String},
			{Name: "ClientEnv", Type: arrow.BinaryTypes.String},
			{Name: "ClientVersion", Type: arrow.BinaryTypes.String},
			{Name: "ClientContainerID", Type: arrow.BinaryTypes.String},
			// Bucket time window (from ClientStatsBucket)
			{Name: "BucketStart", Type: arrow.PrimitiveTypes.Int64},    // ms since epoch
			{Name: "BucketDuration", Type: arrow.PrimitiveTypes.Int64}, // nanoseconds
			// Aggregation dimensions (from ClientGroupedStats)
			{Name: "Service", Type: arrow.BinaryTypes.String},
			{Name: "Name", Type: arrow.BinaryTypes.String}, // operation name
			{Name: "Resource", Type: arrow.BinaryTypes.String},
			{Name: "Type", Type: arrow.BinaryTypes.String},
			{Name: "HTTPStatusCode", Type: arrow.PrimitiveTypes.Uint32},
			{Name: "SpanKind", Type: arrow.BinaryTypes.String},
			{Name: "IsTraceRoot", Type: arrow.PrimitiveTypes.Int32}, // 0=NOT_SET, 1=TRUE, 2=FALSE
			{Name: "Synthetics", Type: arrow.FixedWidthTypes.Boolean},
			// Aggregated values (from ClientGroupedStats)
			{Name: "Hits", Type: arrow.PrimitiveTypes.Uint64},
			{Name: "Errors", Type: arrow.PrimitiveTypes.Uint64},
			{Name: "TopLevelHits", Type: arrow.PrimitiveTypes.Uint64},
			{Name: "Duration", Type: arrow.PrimitiveTypes.Uint64}, // total nanoseconds
			// Latency distributions as DDSketch bytes
			{Name: "OkSummary", Type: arrow.BinaryTypes.Binary},
			{Name: "ErrorSummary", Type: arrow.BinaryTypes.Binary},
			// Peer tags
			{Name: "PeerTags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
		},
		nil,
	)

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Service", true),
		parquet.WithBloomFilterFPPFor("Service", 0.01),
		parquet.WithBloomFilterEnabledFor("Name", true),
		parquet.WithBloomFilterFPPFor("Name", 0.01),
		parquet.WithBloomFilterEnabledFor("Resource", true),
		parquet.WithBloomFilterFPPFor("Resource", 0.01),
	)

	builder := newTraceStatsBatchBuilder(schema)
	pw := &TraceStatsParquetWriter{
		parquetWriterBase: parquetWriterBase{
			outputDir:         outputDir,
			filePrefix:        "observer-trace-stats",
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

	pkglog.Infof("Trace stats parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return pw, nil
}

// WriteStatRow adds one aggregated stat group to the batch.
func (pw *TraceStatsParquetWriter) WriteStatRow(
	source string,
	agentHostname, agentEnv string,
	clientHostname, clientEnv, clientVersion, clientContainerID string,
	bucketStart, bucketDuration uint64,
	service, name, resource, spanType string,
	httpStatusCode uint32,
	spanKind string,
	isTraceRoot int32,
	synthetics bool,
	hits, errors, topLevelHits, duration uint64,
	okSummary, errorSummary []byte,
	peerTags []string,
) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.typedBuilder.add(
		source,
		agentHostname, agentEnv,
		clientHostname, clientEnv, clientVersion, clientContainerID,
		bucketStart, bucketDuration,
		service, name, resource, spanType,
		httpStatusCode, spanKind, isTraceRoot, synthetics,
		hits, errors, topLevelHits, duration,
		okSummary, errorSummary,
		peerTags,
	)
}

// traceStatsBatchBuilder accumulates trace stat rows into Arrow record batches.
type traceStatsBatchBuilder struct {
	schema *arrow.Schema

	runIDs             []string
	agentHostnames     []string
	agentEnvs          []string
	clientHostnames    []string
	clientEnvs         []string
	clientVersions     []string
	clientContainerIDs []string
	bucketStarts       []int64
	bucketDurations    []int64
	services           []string
	names              []string
	resources          []string
	spanTypes          []string
	httpStatusCodes    []uint32
	spanKinds          []string
	isTraceRoots       []int32
	synthetics         []bool
	hits               []uint64
	errors             []uint64
	topLevelHits       []uint64
	durations          []uint64
	okSummaries        [][]byte
	errorSummaries     [][]byte
	peerTags           [][]string
}

func newTraceStatsBatchBuilder(schema *arrow.Schema) *traceStatsBatchBuilder {
	return &traceStatsBatchBuilder{schema: schema}
}

func (b *traceStatsBatchBuilder) add(
	source string,
	agentHostname, agentEnv string,
	clientHostname, clientEnv, clientVersion, clientContainerID string,
	bucketStart, bucketDuration uint64,
	service, name, resource, spanType string,
	httpStatusCode uint32,
	spanKind string,
	isTraceRoot int32,
	synthetics bool,
	hits, errors, topLevelHits, duration uint64,
	okSummary, errorSummary []byte,
	peerTags []string,
) {
	b.runIDs = append(b.runIDs, source)
	b.agentHostnames = append(b.agentHostnames, agentHostname)
	b.agentEnvs = append(b.agentEnvs, agentEnv)
	b.clientHostnames = append(b.clientHostnames, clientHostname)
	b.clientEnvs = append(b.clientEnvs, clientEnv)
	b.clientVersions = append(b.clientVersions, clientVersion)
	b.clientContainerIDs = append(b.clientContainerIDs, clientContainerID)
	b.bucketStarts = append(b.bucketStarts, int64(bucketStart/1000000)) // ns → ms
	b.bucketDurations = append(b.bucketDurations, int64(bucketDuration))
	b.services = append(b.services, service)
	b.names = append(b.names, name)
	b.resources = append(b.resources, resource)
	b.spanTypes = append(b.spanTypes, spanType)
	b.httpStatusCodes = append(b.httpStatusCodes, httpStatusCode)
	b.spanKinds = append(b.spanKinds, spanKind)
	b.isTraceRoots = append(b.isTraceRoots, isTraceRoot)
	b.synthetics = append(b.synthetics, synthetics)
	b.hits = append(b.hits, hits)
	b.errors = append(b.errors, errors)
	b.topLevelHits = append(b.topLevelHits, topLevelHits)
	b.durations = append(b.durations, duration)

	okCopy := make([]byte, len(okSummary))
	copy(okCopy, okSummary)
	b.okSummaries = append(b.okSummaries, okCopy)

	errCopy := make([]byte, len(errorSummary))
	copy(errCopy, errorSummary)
	b.errorSummaries = append(b.errorSummaries, errCopy)

	tagsCopy := make([]string, len(peerTags))
	copy(tagsCopy, peerTags)
	b.peerTags = append(b.peerTags, tagsCopy)
}

func (b *traceStatsBatchBuilder) build() arrow.Record {
	if len(b.runIDs) == 0 {
		return nil
	}

	rb := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	runIDBuilder := rb.Field(0).(*array.StringBuilder)
	agentHostnameBuilder := rb.Field(1).(*array.StringBuilder)
	agentEnvBuilder := rb.Field(2).(*array.StringBuilder)
	clientHostnameBuilder := rb.Field(3).(*array.StringBuilder)
	clientEnvBuilder := rb.Field(4).(*array.StringBuilder)
	clientVersionBuilder := rb.Field(5).(*array.StringBuilder)
	clientContainerIDBuilder := rb.Field(6).(*array.StringBuilder)
	bucketStartBuilder := rb.Field(7).(*array.Int64Builder)
	bucketDurationBuilder := rb.Field(8).(*array.Int64Builder)
	serviceBuilder := rb.Field(9).(*array.StringBuilder)
	nameBuilder := rb.Field(10).(*array.StringBuilder)
	resourceBuilder := rb.Field(11).(*array.StringBuilder)
	spanTypeBuilder := rb.Field(12).(*array.StringBuilder)
	httpStatusBuilder := rb.Field(13).(*array.Uint32Builder)
	spanKindBuilder := rb.Field(14).(*array.StringBuilder)
	isTraceRootBuilder := rb.Field(15).(*array.Int32Builder)
	syntheticsBuilder := rb.Field(16).(*array.BooleanBuilder)
	hitsBuilder := rb.Field(17).(*array.Uint64Builder)
	errorsBuilder := rb.Field(18).(*array.Uint64Builder)
	topLevelHitsBuilder := rb.Field(19).(*array.Uint64Builder)
	durationBuilder := rb.Field(20).(*array.Uint64Builder)
	okSummaryBuilder := rb.Field(21).(*array.BinaryBuilder)
	errorSummaryBuilder := rb.Field(22).(*array.BinaryBuilder)
	peerTagsBuilder := rb.Field(23).(*array.ListBuilder)
	peerTagsValueBuilder := peerTagsBuilder.ValueBuilder().(*array.StringBuilder)

	for _, v := range b.runIDs {
		runIDBuilder.Append(v)
	}
	for _, v := range b.agentHostnames {
		agentHostnameBuilder.Append(v)
	}
	for _, v := range b.agentEnvs {
		agentEnvBuilder.Append(v)
	}
	for _, v := range b.clientHostnames {
		clientHostnameBuilder.Append(v)
	}
	for _, v := range b.clientEnvs {
		clientEnvBuilder.Append(v)
	}
	for _, v := range b.clientVersions {
		clientVersionBuilder.Append(v)
	}
	for _, v := range b.clientContainerIDs {
		clientContainerIDBuilder.Append(v)
	}
	bucketStartBuilder.AppendValues(b.bucketStarts, nil)
	bucketDurationBuilder.AppendValues(b.bucketDurations, nil)
	for _, v := range b.services {
		serviceBuilder.Append(v)
	}
	for _, v := range b.names {
		nameBuilder.Append(v)
	}
	for _, v := range b.resources {
		resourceBuilder.Append(v)
	}
	for _, v := range b.spanTypes {
		spanTypeBuilder.Append(v)
	}
	httpStatusBuilder.AppendValues(b.httpStatusCodes, nil)
	for _, v := range b.spanKinds {
		spanKindBuilder.Append(v)
	}
	isTraceRootBuilder.AppendValues(b.isTraceRoots, nil)
	for _, v := range b.synthetics {
		syntheticsBuilder.Append(v)
	}
	hitsBuilder.AppendValues(b.hits, nil)
	errorsBuilder.AppendValues(b.errors, nil)
	topLevelHitsBuilder.AppendValues(b.topLevelHits, nil)
	durationBuilder.AppendValues(b.durations, nil)
	for _, v := range b.okSummaries {
		okSummaryBuilder.Append(v)
	}
	for _, v := range b.errorSummaries {
		errorSummaryBuilder.Append(v)
	}
	for _, tagList := range b.peerTags {
		peerTagsBuilder.Append(true)
		for _, tag := range tagList {
			peerTagsValueBuilder.Append(tag)
		}
	}

	record := rb.NewRecord()
	rb.Release()

	// Reset builder
	b.runIDs = nil
	b.agentHostnames = nil
	b.agentEnvs = nil
	b.clientHostnames = nil
	b.clientEnvs = nil
	b.clientVersions = nil
	b.clientContainerIDs = nil
	b.bucketStarts = nil
	b.bucketDurations = nil
	b.services = nil
	b.names = nil
	b.resources = nil
	b.spanTypes = nil
	b.httpStatusCodes = nil
	b.spanKinds = nil
	b.isTraceRoots = nil
	b.synthetics = nil
	b.hits = nil
	b.errors = nil
	b.topLevelHits = nil
	b.durations = nil
	b.okSummaries = nil
	b.errorSummaries = nil
	b.peerTags = nil

	return record
}
