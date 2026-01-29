// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
)

// FGMMetric represents a single metric from the FGM parquet file.
type FGMMetric struct {
	RunID      string
	Time       int64 // milliseconds
	MetricName string
	ValueInt   *uint64
	ValueFloat *float64
	Tags       map[string]string
}

// ParquetReader reads FGM parquet files and provides metrics in chronological order.
type ParquetReader struct {
	metrics []FGMMetric
	index   int
}

// NewParquetReader creates a new parquet reader from a directory containing FGM parquet files.
func NewParquetReader(dirPath string) (*ParquetReader, error) {
	// Find all parquet files in the directory
	parquetFiles, err := findParquetFiles(dirPath)
	if err != nil {
		return nil, fmt.Errorf("finding parquet files: %w", err)
	}

	if len(parquetFiles) == 0 {
		return nil, fmt.Errorf("no parquet files found in %s", dirPath)
	}

	// Read all metrics from all files
	var allMetrics []FGMMetric
	for _, filePath := range parquetFiles {
		metrics, err := readParquetFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", filePath, err)
		}
		allMetrics = append(allMetrics, metrics...)
	}

	// Sort by timestamp
	sort.Slice(allMetrics, func(i, j int) bool {
		return allMetrics[i].Time < allMetrics[j].Time
	})

	return &ParquetReader{
		metrics: allMetrics,
		index:   0,
	}, nil
}

// Next returns the next metric, or nil if no more metrics.
func (r *ParquetReader) Next() *FGMMetric {
	if r.index >= len(r.metrics) {
		return nil
	}
	metric := &r.metrics[r.index]
	r.index++
	return metric
}

// Reset resets the reader to the beginning.
func (r *ParquetReader) Reset() {
	r.index = 0
}

// Len returns the total number of metrics.
func (r *ParquetReader) Len() int {
	return len(r.metrics)
}

// StartTime returns the timestamp of the first metric in milliseconds.
func (r *ParquetReader) StartTime() int64 {
	if len(r.metrics) == 0 {
		return 0
	}
	return r.metrics[0].Time
}

// EndTime returns the timestamp of the last metric in milliseconds.
func (r *ParquetReader) EndTime() int64 {
	if len(r.metrics) == 0 {
		return 0
	}
	return r.metrics[len(r.metrics)-1].Time
}

// findParquetFiles recursively finds all .parquet files in a directory.
func findParquetFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".parquet") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files) // Sort for consistent ordering
	return files, nil
}

// readParquetFile reads a single parquet file and extracts FGM metrics.
func readParquetFile(filePath string) ([]FGMMetric, error) {
	// Open the parquet file
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	// Create parquet file reader
	parquetReader, err := file.NewParquetReader(f)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}
	defer parquetReader.Close()

	// Create arrow file reader from parquet reader
	reader, err := pqarrow.NewFileReader(parquetReader, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		return nil, fmt.Errorf("creating arrow reader: %w", err)
	}

	// Read the entire table
	ctx := context.Background()
	table, err := reader.ReadTable(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading table: %w", err)
	}
	defer table.Release()

	// Extract metrics from the table
	return extractMetricsFromTable(table)
}

// extractMetricsFromTable extracts FGMMetric structs from an Arrow table.
func extractMetricsFromTable(table arrow.Table) ([]FGMMetric, error) {
	schema := table.Schema()
	numRows := int(table.NumRows())

	if numRows == 0 {
		return nil, nil
	}

	// Find column indices (case-insensitive)
	runIDIdx := findColumnIndex(schema, "runid")
	timeIdx := findColumnIndex(schema, "time")
	metricNameIdx := findColumnIndex(schema, "metricname")
	valueFloatIdx := findColumnIndex(schema, "valuefloat")
	tagsIdx := findColumnIndex(schema, "tags")

	// Extract column data
	runIDs := getStringColumn(table, runIDIdx)
	times := getInt64Column(table, timeIdx)
	metricNames := getStringColumn(table, metricNameIdx)
	valueFloats := getFloat64Column(table, valueFloatIdx)
	tagsList := getStringListColumn(table, tagsIdx)

	// Build metrics
	metrics := make([]FGMMetric, numRows)
	for i := 0; i < numRows; i++ {
		tags := make(map[string]string)

		// Parse tags from list column
		if i < len(tagsList) {
			for _, tag := range tagsList[i] {
				// Parse "key:value" format
				parts := strings.SplitN(tag, ":", 2)
				if len(parts) == 2 {
					tags[parts[0]] = parts[1]
				} else if len(parts) == 1 && parts[0] != "" {
					tags[parts[0]] = ""
				}
			}
		}

		var valueFloat *float64
		if i < len(valueFloats) && valueFloats[i] != 0 {
			v := valueFloats[i]
			valueFloat = &v
		}

		metrics[i] = FGMMetric{
			RunID:      getAt(runIDs, i),
			Time:       times[i],
			MetricName: getAt(metricNames, i),
			ValueInt:   nil, // Not used in new schema
			ValueFloat: valueFloat,
			Tags:       tags,
		}
	}

	return metrics, nil
}

// findColumnIndex finds the index of a column by name (case-insensitive), returns -1 if not found.
func findColumnIndex(schema *arrow.Schema, name string) int {
	nameLower := strings.ToLower(name)
	for i, field := range schema.Fields() {
		if strings.ToLower(field.Name) == nameLower {
			return i
		}
	}
	return -1
}

// getStringColumn extracts a string column from an Arrow table.
func getStringColumn(table arrow.Table, colIdx int) []string {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([]string, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		strArr, ok := chunk.(*array.String)
		if !ok {
			binaryArr, ok := chunk.(*array.Binary)
			if ok {
				// Handle binary arrays (ByteArray in Parquet)
				for i := 0; i < binaryArr.Len(); i++ {
					if !binaryArr.IsNull(i) {
						result[offset+i] = string(binaryArr.Value(i))
					}
				}
				offset += binaryArr.Len()
				continue
			}
			offset += chunk.Len()
			continue
		}

		for i := 0; i < strArr.Len(); i++ {
			if !strArr.IsNull(i) {
				result[offset+i] = strArr.Value(i)
			}
		}
		offset += strArr.Len()
	}

	return result
}

// getInt64Column extracts an int64 column from an Arrow table.
func getInt64Column(table arrow.Table, colIdx int) []int64 {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([]int64, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		i64Arr, ok := chunk.(*array.Int64)
		if !ok {
			// Try as Timestamp and convert to milliseconds
			tsArr, ok := chunk.(*array.Timestamp)
			if ok {
				for i := 0; i < tsArr.Len(); i++ {
					if !tsArr.IsNull(i) {
						result[offset+i] = int64(tsArr.Value(i))
					}
				}
				offset += tsArr.Len()
				continue
			}
			offset += chunk.Len()
			continue
		}

		for i := 0; i < i64Arr.Len(); i++ {
			if !i64Arr.IsNull(i) {
				result[offset+i] = i64Arr.Value(i)
			}
		}
		offset += i64Arr.Len()
	}

	return result
}

// getFloat64Column extracts a float64 column from an Arrow table.
func getFloat64Column(table arrow.Table, colIdx int) []float64 {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([]float64, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		f64Arr, ok := chunk.(*array.Float64)
		if !ok {
			offset += chunk.Len()
			continue
		}

		for i := 0; i < f64Arr.Len(); i++ {
			if !f64Arr.IsNull(i) {
				result[offset+i] = f64Arr.Value(i)
			}
		}
		offset += f64Arr.Len()
	}

	return result
}

// getStringListColumn extracts a list<string> column from an Arrow table.
func getStringListColumn(table arrow.Table, colIdx int) [][]string {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([][]string, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		listArr, ok := chunk.(*array.List)
		if !ok {
			offset += chunk.Len()
			continue
		}

		// Get the values array (contains all strings)
		valuesArr := listArr.ListValues().(*array.String)

		for i := 0; i < listArr.Len(); i++ {
			if listArr.IsNull(i) {
				result[offset+i] = []string{}
				continue
			}

			// Get start and end offsets for this list
			start := int(listArr.Offsets()[i])
			end := int(listArr.Offsets()[i+1])

			// Extract strings for this list
			tags := make([]string, 0, end-start)
			for j := start; j < end; j++ {
				if !valuesArr.IsNull(j) {
					tags = append(tags, valuesArr.Value(j))
				}
			}
			result[offset+i] = tags
		}
		offset += listArr.Len()
	}

	return result
}

// Helper functions to safely get values from slices
func getAt(slice []string, idx int) string {
	if idx < len(slice) {
		return slice[idx]
	}
	return ""
}
