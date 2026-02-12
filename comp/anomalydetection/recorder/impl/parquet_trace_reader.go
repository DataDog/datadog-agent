// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// TraceParquetReader reads trace data from parquet files.
// Traces are stored as denormalized spans (one row per span) and are
// reconstructed by grouping spans by trace ID.
type TraceParquetReader struct {
	inputDir string
	files    []string
}

// NewTraceParquetReader creates a reader for trace parquet files.
func NewTraceParquetReader(inputDir string) (*TraceParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "observer-traces-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(inputDir, entry.Name()))
		}
	}

	// Sort files by name (which includes timestamp) for chronological order
	sort.Strings(files)

	return &TraceParquetReader{
		inputDir: inputDir,
		files:    files,
	}, nil
}

// traceKey is used to group spans by trace ID.
type traceKey struct {
	high, low uint64
}

// ReadAll reads all traces from all parquet files and reconstructs them from spans.
func (r *TraceParquetReader) ReadAll() []recorderdef.TraceData {
	// Map to group spans by trace ID (high:low as key)
	traceMap := make(map[traceKey]*recorderdef.TraceData)

	for _, filePath := range r.files {
		r.readFile(filePath, traceMap)
	}

	// Convert map to slice
	traces := make([]recorderdef.TraceData, 0, len(traceMap))
	for _, trace := range traceMap {
		traces = append(traces, *trace)
	}

	// Sort by timestamp for consistent ordering
	sort.Slice(traces, func(i, j int) bool {
		return traces[i].Timestamp < traces[j].Timestamp
	})

	return traces
}

func (r *TraceParquetReader) readFile(filePath string, traceMap map[traceKey]*recorderdef.TraceData) {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open trace parquet file %s: %v", filePath, err)
		return
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		pkglog.Warnf("Failed to create parquet reader for %s: %v", filePath, err)
		return
	}
	defer pf.Close()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		pkglog.Warnf("Failed to create arrow reader for %s: %v", filePath, err)
		return
	}

	table, err := reader.ReadTable(context.TODO())
	if err != nil {
		pkglog.Warnf("Failed to read table from %s: %v", filePath, err)
		return
	}
	defer table.Release()

	numRows := int(table.NumRows())
	if numRows == 0 {
		return
	}

	// Get column indices
	schema := table.Schema()
	colIndices := make(map[string]int)
	for i, field := range schema.Fields() {
		colIndices[field.Name] = i
	}

	// Read columns
	getStringCol := func(name string) *array.String {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.String)
			}
		}
		return nil
	}
	getInt64Col := func(name string) *array.Int64 {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Int64)
			}
		}
		return nil
	}
	getUint64Col := func(name string) *array.Uint64 {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Uint64)
			}
		}
		return nil
	}
	getInt32Col := func(name string) *array.Int32 {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Int32)
			}
		}
		return nil
	}
	getBoolCol := func(name string) *array.Boolean {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Boolean)
			}
		}
		return nil
	}
	getStringListCol := func(name string) *array.List {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.List)
			}
		}
		return nil
	}

	// Trace-level columns
	runIDCol := getStringCol("RunID")
	traceIDHighCol := getUint64Col("TraceIDHigh")
	traceIDLowCol := getUint64Col("TraceIDLow")
	envCol := getStringCol("Env")
	traceServiceCol := getStringCol("TraceService")
	hostnameCol := getStringCol("Hostname")
	containerIDCol := getStringCol("ContainerID")
	traceDurationCol := getInt64Col("TraceDurationNs")
	priorityCol := getInt32Col("Priority")
	traceErrorCol := getBoolCol("TraceError")
	traceTagsCol := getStringListCol("TraceTags")
	timeCol := getInt64Col("Time")

	// Span-level columns
	spanIDCol := getUint64Col("SpanID")
	parentIDCol := getUint64Col("ParentID")
	spanServiceCol := getStringCol("SpanService")
	spanNameCol := getStringCol("SpanName")
	resourceCol := getStringCol("Resource")
	spanTypeCol := getStringCol("SpanType")
	spanStartCol := getInt64Col("SpanStartNs")
	spanDurationCol := getInt64Col("SpanDurationNs")
	spanErrorCol := getInt32Col("SpanError")
	spanMetaCol := getStringListCol("SpanMeta")
	spanMetricsCol := getStringListCol("SpanMetrics")

	for i := 0; i < numRows; i++ {
		traceIDHigh := traceIDHighCol.Value(i)
		traceIDLow := traceIDLowCol.Value(i)
		key := traceKey{traceIDHigh, traceIDLow}

		// Get or create trace
		trace, exists := traceMap[key]
		if !exists {
			trace = &recorderdef.TraceData{
				TraceIDHigh: traceIDHigh,
				TraceIDLow:  traceIDLow,
			}
			if runIDCol != nil {
				trace.Source = runIDCol.Value(i)
			}
			if envCol != nil {
				trace.Env = envCol.Value(i)
			}
			if traceServiceCol != nil {
				trace.Service = traceServiceCol.Value(i)
			}
			if hostnameCol != nil {
				trace.Hostname = hostnameCol.Value(i)
			}
			if containerIDCol != nil {
				trace.ContainerID = containerIDCol.Value(i)
			}
			if timeCol != nil {
				trace.Timestamp = timeCol.Value(i) * 1000000 // Convert ms to ns
			}
			if traceDurationCol != nil {
				trace.Duration = traceDurationCol.Value(i)
			}
			if priorityCol != nil {
				trace.Priority = priorityCol.Value(i)
			}
			if traceErrorCol != nil {
				trace.IsError = traceErrorCol.Value(i)
			}
			if traceTagsCol != nil {
				trace.Tags = readStringList(traceTagsCol, i)
			}
			traceMap[key] = trace
		}

		// Add span
		span := recorderdef.SpanData{}
		if spanIDCol != nil {
			span.SpanID = spanIDCol.Value(i)
		}
		if parentIDCol != nil {
			span.ParentID = parentIDCol.Value(i)
		}
		if spanServiceCol != nil {
			span.Service = spanServiceCol.Value(i)
		}
		if spanNameCol != nil {
			span.Name = spanNameCol.Value(i)
		}
		if resourceCol != nil {
			span.Resource = resourceCol.Value(i)
		}
		if spanTypeCol != nil {
			span.Type = spanTypeCol.Value(i)
		}
		if spanStartCol != nil {
			span.Start = spanStartCol.Value(i)
		}
		if spanDurationCol != nil {
			span.Duration = spanDurationCol.Value(i)
		}
		if spanErrorCol != nil {
			span.Error = spanErrorCol.Value(i)
		}
		if spanMetaCol != nil {
			span.Meta = readStringList(spanMetaCol, i)
		}
		if spanMetricsCol != nil {
			span.Metrics = readStringList(spanMetricsCol, i)
		}

		trace.Spans = append(trace.Spans, span)
	}
}

// readStringList reads a string list from an Arrow List column at the given row.
func readStringList(col *array.List, row int) []string {
	if col.IsNull(row) {
		return nil
	}
	start, end := col.ValueOffsets(row)
	values := col.ListValues().(*array.String)
	result := make([]string, end-start)
	for j := start; j < end; j++ {
		result[j-start] = values.Value(int(j))
	}
	return result
}
