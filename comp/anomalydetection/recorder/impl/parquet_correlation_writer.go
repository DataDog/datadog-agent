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

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// correlationParquetWriter writes correlator output to parquet files.
// Each row represents one ActiveCorrelation with its member series and embedded anomaly
// details stored as parallel lists, enabling full reconstruction without joining other files.
// Files are only created when there is data to write; empty files are never produced.
type correlationParquetWriter struct {
	parquetWriter
	typedBuilder *correlationBatchBuilder
}

// newResultsCorrelationParquetWriter creates a writer for correlator output
// (observer-resultscorrelations-*.parquet).
func newResultsCorrelationParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*correlationParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	schema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "Pattern", Type: arrow.BinaryTypes.String},
			{Name: "Title", Type: arrow.BinaryTypes.String},
			{Name: "FirstSeen", Type: arrow.PrimitiveTypes.Int64},   // unix seconds
			{Name: "LastUpdated", Type: arrow.PrimitiveTypes.Int64}, // unix seconds
			// Member series and metric names participating in this correlation
			{Name: "MemberSeriesIDs", Type: arrow.ListOf(arrow.BinaryTypes.String)},
			{Name: "MetricNames", Type: arrow.ListOf(arrow.BinaryTypes.String)},
			// Anomaly details as parallel lists (one entry per triggering anomaly)
			{Name: "AnomalyTimestamps", Type: arrow.ListOf(arrow.PrimitiveTypes.Int64)},
			{Name: "AnomalySources", Type: arrow.ListOf(arrow.BinaryTypes.String)},
			{Name: "AnomalySeriesIDs", Type: arrow.ListOf(arrow.BinaryTypes.String)},
			{Name: "AnomalyDetectors", Type: arrow.ListOf(arrow.BinaryTypes.String)},
		},
		nil,
	)

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Pattern", true),
		parquet.WithBloomFilterFPPFor("Pattern", 0.01),
	)

	builder := newCorrelationBatchBuilder(schema)
	cw := &correlationParquetWriter{
		parquetWriter: parquetWriter{
			outputDir:         outputDir,
			filePrefix:        "observer-resultscorrelations",
			schema:            schema,
			writerProps:       props,
			builder:           builder,
			flushInterval:     flushInterval,
			retentionDuration: retentionDuration,
			stopCh:            make(chan struct{}),
		},
		typedBuilder: builder,
	}
	cw.start()

	pkglog.Infof("Correlation parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)
	return cw, nil
}

// WriteCorrelation adds a correlation to the batch (will be flushed on interval or explicit flush).
func (cw *correlationParquetWriter) WriteCorrelation(data recorderdef.CorrelationData) {
	cw.mu.Lock()
	defer cw.mu.Unlock()
	cw.typedBuilder.add(data)
}

// correlationBatchBuilder accumulates correlation rows into Arrow record batches.
type correlationBatchBuilder struct {
	schema *arrow.Schema

	patterns    []string
	titles      []string
	firstSeens  []int64
	lastUpdates []int64

	memberSeriesIDs [][]string
	metricNames     [][]string

	anomalyTimestamps [][]int64
	anomalySources    [][]string
	anomalySeriesIDs  [][]string
	anomalyDetectors  [][]string
}

func newCorrelationBatchBuilder(schema *arrow.Schema) *correlationBatchBuilder {
	return &correlationBatchBuilder{schema: schema}
}

func (b *correlationBatchBuilder) add(data recorderdef.CorrelationData) {
	b.patterns = append(b.patterns, data.Pattern)
	b.titles = append(b.titles, data.Title)
	b.firstSeens = append(b.firstSeens, data.FirstSeen)
	b.lastUpdates = append(b.lastUpdates, data.LastUpdated)

	memberCopy := make([]string, len(data.MemberSeriesIDs))
	copy(memberCopy, data.MemberSeriesIDs)
	b.memberSeriesIDs = append(b.memberSeriesIDs, memberCopy)

	metricCopy := make([]string, len(data.MetricNames))
	copy(metricCopy, data.MetricNames)
	b.metricNames = append(b.metricNames, metricCopy)

	tsCopy := make([]int64, len(data.AnomalyTimestamps))
	copy(tsCopy, data.AnomalyTimestamps)
	b.anomalyTimestamps = append(b.anomalyTimestamps, tsCopy)

	srcCopy := make([]string, len(data.AnomalySources))
	copy(srcCopy, data.AnomalySources)
	b.anomalySources = append(b.anomalySources, srcCopy)

	sidCopy := make([]string, len(data.AnomalySeriesIDs))
	copy(sidCopy, data.AnomalySeriesIDs)
	b.anomalySeriesIDs = append(b.anomalySeriesIDs, sidCopy)

	detCopy := make([]string, len(data.AnomalyDetectors))
	copy(detCopy, data.AnomalyDetectors)
	b.anomalyDetectors = append(b.anomalyDetectors, detCopy)
}

func (b *correlationBatchBuilder) build() arrow.Record {
	if len(b.patterns) == 0 {
		return nil
	}

	rb := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	patternBuilder := rb.Field(0).(*array.StringBuilder)
	titleBuilder := rb.Field(1).(*array.StringBuilder)
	firstSeenBuilder := rb.Field(2).(*array.Int64Builder)
	lastUpdatedBuilder := rb.Field(3).(*array.Int64Builder)

	memberSeriesBuilder := rb.Field(4).(*array.ListBuilder)
	memberSeriesValueBuilder := memberSeriesBuilder.ValueBuilder().(*array.StringBuilder)

	metricNamesBuilder := rb.Field(5).(*array.ListBuilder)
	metricNamesValueBuilder := metricNamesBuilder.ValueBuilder().(*array.StringBuilder)

	anomalyTsBuilder := rb.Field(6).(*array.ListBuilder)
	anomalyTsValueBuilder := anomalyTsBuilder.ValueBuilder().(*array.Int64Builder)

	anomalySrcBuilder := rb.Field(7).(*array.ListBuilder)
	anomalySrcValueBuilder := anomalySrcBuilder.ValueBuilder().(*array.StringBuilder)

	anomalySidBuilder := rb.Field(8).(*array.ListBuilder)
	anomalySidValueBuilder := anomalySidBuilder.ValueBuilder().(*array.StringBuilder)

	anomalyDetBuilder := rb.Field(9).(*array.ListBuilder)
	anomalyDetValueBuilder := anomalyDetBuilder.ValueBuilder().(*array.StringBuilder)

	for _, v := range b.patterns {
		patternBuilder.Append(v)
	}
	for _, v := range b.titles {
		titleBuilder.Append(v)
	}
	firstSeenBuilder.AppendValues(b.firstSeens, nil)
	lastUpdatedBuilder.AppendValues(b.lastUpdates, nil)

	for _, list := range b.memberSeriesIDs {
		memberSeriesBuilder.Append(true)
		for _, v := range list {
			memberSeriesValueBuilder.Append(v)
		}
	}
	for _, list := range b.metricNames {
		metricNamesBuilder.Append(true)
		for _, v := range list {
			metricNamesValueBuilder.Append(v)
		}
	}
	for _, list := range b.anomalyTimestamps {
		anomalyTsBuilder.Append(true)
		anomalyTsValueBuilder.AppendValues(list, nil)
	}
	for _, list := range b.anomalySources {
		anomalySrcBuilder.Append(true)
		for _, v := range list {
			anomalySrcValueBuilder.Append(v)
		}
	}
	for _, list := range b.anomalySeriesIDs {
		anomalySidBuilder.Append(true)
		for _, v := range list {
			anomalySidValueBuilder.Append(v)
		}
	}
	for _, list := range b.anomalyDetectors {
		anomalyDetBuilder.Append(true)
		for _, v := range list {
			anomalyDetValueBuilder.Append(v)
		}
	}

	record := rb.NewRecord()
	rb.Release()

	b.patterns = nil
	b.titles = nil
	b.firstSeens = nil
	b.lastUpdates = nil
	b.memberSeriesIDs = nil
	b.metricNames = nil
	b.anomalyTimestamps = nil
	b.anomalySources = nil
	b.anomalySeriesIDs = nil
	b.anomalyDetectors = nil

	return record
}
