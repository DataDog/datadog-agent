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

// TraceStatsParquetReader reads APM trace stats from parquet files.
type TraceStatsParquetReader struct {
	inputDir string
	files    []string
}

// NewTraceStatsParquetReader creates a reader for trace stats parquet files.
func NewTraceStatsParquetReader(inputDir string) (*TraceStatsParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "observer-trace-stats-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(inputDir, entry.Name()))
		}
	}

	sort.Strings(files)

	return &TraceStatsParquetReader{
		inputDir: inputDir,
		files:    files,
	}, nil
}

// ReadAll reads all trace stat rows from all parquet files.
func (r *TraceStatsParquetReader) ReadAll() []recorderdef.TraceStatsData {
	var stats []recorderdef.TraceStatsData

	for _, filePath := range r.files {
		r.readFile(filePath, &stats)
	}

	// Sort by bucket start for consistent ordering
	sort.Slice(stats, func(i, j int) bool {
		return stats[i].BucketStart < stats[j].BucketStart
	})

	return stats
}

func (r *TraceStatsParquetReader) readFile(filePath string, stats *[]recorderdef.TraceStatsData) {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open trace stats parquet file %s: %v", filePath, err)
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

	schema := table.Schema()
	colIndices := make(map[string]int)
	for i, field := range schema.Fields() {
		colIndices[field.Name] = i
	}

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
	getUint32Col := func(name string) *array.Uint32 {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Uint32)
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
	getUint64Col := func(name string) *array.Uint64 {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Uint64)
			}
		}
		return nil
	}
	getBinaryCol := func(name string) *array.Binary {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Binary)
			}
		}
		return nil
	}
	getListCol := func(name string) *array.List {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.List)
			}
		}
		return nil
	}

	runIDCol := getStringCol("RunID")
	agentHostnameCol := getStringCol("AgentHostname")
	agentEnvCol := getStringCol("AgentEnv")
	clientHostnameCol := getStringCol("ClientHostname")
	clientEnvCol := getStringCol("ClientEnv")
	clientVersionCol := getStringCol("ClientVersion")
	clientContainerIDCol := getStringCol("ClientContainerID")
	bucketStartCol := getInt64Col("BucketStart")
	bucketDurationCol := getInt64Col("BucketDuration")
	serviceCol := getStringCol("Service")
	nameCol := getStringCol("Name")
	resourceCol := getStringCol("Resource")
	spanTypeCol := getStringCol("Type")
	httpStatusCol := getUint32Col("HTTPStatusCode")
	spanKindCol := getStringCol("SpanKind")
	isTraceRootCol := getInt32Col("IsTraceRoot")
	syntheticsCol := getBoolCol("Synthetics")
	hitsCol := getUint64Col("Hits")
	errorsCol := getUint64Col("Errors")
	topLevelHitsCol := getUint64Col("TopLevelHits")
	durationCol := getUint64Col("Duration")
	okSummaryCol := getBinaryCol("OkSummary")
	errorSummaryCol := getBinaryCol("ErrorSummary")
	peerTagsCol := getListCol("PeerTags")

	for i := 0; i < numRows; i++ {
		row := recorderdef.TraceStatsData{}

		if runIDCol != nil {
			row.Source = runIDCol.Value(i)
		}
		if agentHostnameCol != nil {
			row.AgentHostname = agentHostnameCol.Value(i)
		}
		if agentEnvCol != nil {
			row.AgentEnv = agentEnvCol.Value(i)
		}
		if clientHostnameCol != nil {
			row.ClientHostname = clientHostnameCol.Value(i)
		}
		if clientEnvCol != nil {
			row.ClientEnv = clientEnvCol.Value(i)
		}
		if clientVersionCol != nil {
			row.ClientVersion = clientVersionCol.Value(i)
		}
		if clientContainerIDCol != nil {
			row.ClientContainerID = clientContainerIDCol.Value(i)
		}
		if bucketStartCol != nil {
			// Stored as ms, convert back to ns
			row.BucketStart = uint64(bucketStartCol.Value(i)) * 1_000_000
		}
		if bucketDurationCol != nil {
			row.BucketDuration = uint64(bucketDurationCol.Value(i))
		}
		if serviceCol != nil {
			row.Service = serviceCol.Value(i)
		}
		if nameCol != nil {
			row.Name = nameCol.Value(i)
		}
		if resourceCol != nil {
			row.Resource = resourceCol.Value(i)
		}
		if spanTypeCol != nil {
			row.Type = spanTypeCol.Value(i)
		}
		if httpStatusCol != nil {
			row.HTTPStatusCode = httpStatusCol.Value(i)
		}
		if spanKindCol != nil {
			row.SpanKind = spanKindCol.Value(i)
		}
		if isTraceRootCol != nil {
			row.IsTraceRoot = isTraceRootCol.Value(i)
		}
		if syntheticsCol != nil {
			row.Synthetics = syntheticsCol.Value(i)
		}
		if hitsCol != nil {
			row.Hits = hitsCol.Value(i)
		}
		if errorsCol != nil {
			row.Errors = errorsCol.Value(i)
		}
		if topLevelHitsCol != nil {
			row.TopLevelHits = topLevelHitsCol.Value(i)
		}
		if durationCol != nil {
			row.Duration = durationCol.Value(i)
		}
		if okSummaryCol != nil && !okSummaryCol.IsNull(i) {
			data := okSummaryCol.Value(i)
			row.OkSummary = make([]byte, len(data))
			copy(row.OkSummary, data)
		}
		if errorSummaryCol != nil && !errorSummaryCol.IsNull(i) {
			data := errorSummaryCol.Value(i)
			row.ErrorSummary = make([]byte, len(data))
			copy(row.ErrorSummary, data)
		}
		if peerTagsCol != nil {
			row.PeerTags = readStringList(peerTagsCol, i)
		}

		*stats = append(*stats, row)
	}
}
