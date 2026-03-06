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
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// correlationParquetReader reads correlation data from parquet files.
type correlationParquetReader struct {
	files []string
}

// newResultCorrelationParquetReader creates a reader for result correlation parquet files
// (observer-resultscorrelations-*.parquet). Returns an empty reader (no error) when
// no result correlation files are found.
func newResultCorrelationParquetReader(inputDir string) (*correlationParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		if os.IsNotExist(err) {
			return &correlationParquetReader{}, nil
		}
		return nil, err
	}

	const prefix = "observer-resultscorrelations-"
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), prefix) && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(inputDir, entry.Name()))
		}
	}

	sort.Strings(files)
	return &correlationParquetReader{files: files}, nil
}

// ReadAll reads all correlations from all parquet files.
func (r *correlationParquetReader) ReadAll() []recorderdef.CorrelationData {
	var correlations []recorderdef.CorrelationData
	for _, filePath := range r.files {
		r.readFile(filePath, &correlations)
	}
	return correlations
}

func (r *correlationParquetReader) readFile(filePath string, out *[]recorderdef.CorrelationData) {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open correlation parquet file %s: %v", filePath, err)
		return
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		pkglog.Warnf("Failed to create parquet reader for %s: %v", filePath, err)
		return
	}
	defer pf.Close()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
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
	colIdx := make(map[string]int)
	for i, field := range schema.Fields() {
		colIdx[field.Name] = i
	}

	getStringCol := func(name string) *array.String {
		if idx, ok := colIdx[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.String)
			}
		}
		return nil
	}
	getInt64Col := func(name string) *array.Int64 {
		if idx, ok := colIdx[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Int64)
			}
		}
		return nil
	}
	getListCol := func(name string) *array.List {
		if idx, ok := colIdx[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.List)
			}
		}
		return nil
	}

	patternCol := getStringCol("Pattern")
	titleCol := getStringCol("Title")
	firstSeenCol := getInt64Col("FirstSeen")
	lastUpdatedCol := getInt64Col("LastUpdated")
	memberSeriesIDsCol := getListCol("MemberSeriesIDs")
	metricNamesCol := getListCol("MetricNames")
	anomalyTimestampsCol := getListCol("AnomalyTimestamps")
	anomalySourcesCol := getListCol("AnomalySources")
	anomalySeriesIDsCol := getListCol("AnomalySeriesIDs")
	anomalyDetectorsCol := getListCol("AnomalyDetectors")

	for i := 0; i < numRows; i++ {
		cd := recorderdef.CorrelationData{}

		if patternCol != nil {
			cd.Pattern = patternCol.Value(i)
		}
		if titleCol != nil {
			cd.Title = titleCol.Value(i)
		}
		if firstSeenCol != nil {
			cd.FirstSeen = firstSeenCol.Value(i)
		}
		if lastUpdatedCol != nil {
			cd.LastUpdated = lastUpdatedCol.Value(i)
		}
		if memberSeriesIDsCol != nil {
			cd.MemberSeriesIDs = readStringList(memberSeriesIDsCol, i)
		}
		if metricNamesCol != nil {
			cd.MetricNames = readStringList(metricNamesCol, i)
		}
		if anomalyTimestampsCol != nil {
			cd.AnomalyTimestamps = readInt64List(anomalyTimestampsCol, i)
		}
		if anomalySourcesCol != nil {
			cd.AnomalySources = readStringList(anomalySourcesCol, i)
		}
		if anomalySeriesIDsCol != nil {
			cd.AnomalySeriesIDs = readStringList(anomalySeriesIDsCol, i)
		}
		if anomalyDetectorsCol != nil {
			cd.AnomalyDetectors = readStringList(anomalyDetectorsCol, i)
		}

		*out = append(*out, cd)
	}
}

// readInt64List reads an int64 list from an Arrow List column at the given row.
func readInt64List(col *array.List, row int) []int64 {
	if col.IsNull(row) {
		return nil
	}
	start, end := col.ValueOffsets(row)
	values := col.ListValues().(*array.Int64)
	result := make([]int64, end-start)
	for j := start; j < end; j++ {
		result[j-start] = values.Value(int(j))
	}
	return result
}
