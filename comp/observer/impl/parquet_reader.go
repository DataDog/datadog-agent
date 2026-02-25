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

	// Read all metrics from all files, skipping any that fail to parse.
	var allMetrics []FGMMetric
	for _, filePath := range parquetFiles {
		metrics, err := readParquetFile(filePath)
		if err != nil {
			fmt.Printf("[parquet-reader] Skipping %s: %v\n", filePath, err)
			continue
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

// minParquetFileSize is the minimum valid size for a parquet file
const minParquetFileSize = 20

// findParquetFiles recursively finds all .parquet files in a directory,
// skipping files that are too small to be valid parquet files.
func findParquetFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".parquet") {
			if info.Size() < minParquetFileSize {
				fmt.Printf("[parquet-reader] Skipping %s: file too small (%d bytes)\n", path, info.Size())
				return nil
			}
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

	// Find column indices
	runIDIdx := findColumnIndex(schema, "run_id")
	timeIdx := findColumnIndex(schema, "time")
	metricNameIdx := findColumnIndex(schema, "metric_name")
	valueIntIdx := findColumnIndex(schema, "value_int")
	valueFloatIdx := findColumnIndex(schema, "value_float")

	// Find label columns (all columns starting with "l_")
	labelIndices := make(map[string]int)
	for i := 0; i < len(schema.Fields()); i++ {
		name := schema.Field(i).Name
		if strings.HasPrefix(name, "l_") {
			labelIndices[name] = i
		}
	}

	// Extract column data
	runIDs := getStringColumn(table, runIDIdx)
	times := getTimestampColumn(table, timeIdx)
	metricNames := getStringColumn(table, metricNameIdx)
	valueInts := getUInt64Column(table, valueIntIdx)
	valueFloats := getFloat64Column(table, valueFloatIdx)

	// Extract label columns
	labels := make(map[string][]string)
	for name, idx := range labelIndices {
		labels[name] = getStringColumn(table, idx)
	}

	// Build metrics
	metrics := make([]FGMMetric, numRows)
	for i := 0; i < numRows; i++ {
		tags := make(map[string]string)
		for name, values := range labels {
			if i < len(values) && values[i] != "" {
				// Remove "l_" prefix from tag name
				tagName := strings.TrimPrefix(name, "l_")
				tags[tagName] = values[i]
			}
		}

		metrics[i] = FGMMetric{
			RunID:      getAt(runIDs, i),
			Time:       times[i],
			MetricName: getAt(metricNames, i),
			ValueInt:   getUInt64PtrAt(valueInts, i),
			ValueFloat: getFloat64PtrAt(valueFloats, i),
			Tags:       tags,
		}
	}

	return metrics, nil
}

// findColumnIndex finds the index of a column by name, returns -1 if not found.
func findColumnIndex(schema *arrow.Schema, name string) int {
	for i, field := range schema.Fields() {
		if field.Name == name {
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

// getTimestampColumn extracts a timestamp column (as milliseconds) from an Arrow table.
func getTimestampColumn(table arrow.Table, colIdx int) []int64 {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([]int64, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		tsArr, ok := chunk.(*array.Timestamp)
		if !ok {
			offset += chunk.Len()
			continue
		}

		for i := 0; i < tsArr.Len(); i++ {
			if !tsArr.IsNull(i) {
				result[offset+i] = int64(tsArr.Value(i))
			}
		}
		offset += tsArr.Len()
	}

	return result
}

// getUInt64Column extracts a uint64 column from an Arrow table.
func getUInt64Column(table arrow.Table, colIdx int) []uint64 {
	if colIdx < 0 || int64(colIdx) >= table.NumCols() {
		return nil
	}

	col := table.Column(colIdx)
	numRows := int(col.Len())
	result := make([]uint64, numRows)

	// Iterate through all chunks
	offset := 0
	for _, chunk := range col.Data().Chunks() {
		u64Arr, ok := chunk.(*array.Uint64)
		if !ok {
			// Try as Int64 and convert
			i64Arr, ok := chunk.(*array.Int64)
			if ok {
				for i := 0; i < i64Arr.Len(); i++ {
					if !i64Arr.IsNull(i) {
						result[offset+i] = uint64(i64Arr.Value(i))
					}
				}
				offset += i64Arr.Len()
				continue
			}
			offset += chunk.Len()
			continue
		}

		for i := 0; i < u64Arr.Len(); i++ {
			if !u64Arr.IsNull(i) {
				result[offset+i] = u64Arr.Value(i)
			}
		}
		offset += u64Arr.Len()
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

// Helper functions to safely get values from slices
func getAt(slice []string, idx int) string {
	if idx < len(slice) {
		return slice[idx]
	}
	return ""
}

func getUInt64PtrAt(slice []uint64, idx int) *uint64 {
	if idx < len(slice) {
		v := slice[idx]
		return &v
	}
	return nil
}

func getFloat64PtrAt(slice []float64, idx int) *float64 {
	if idx < len(slice) {
		v := slice[idx]
		return &v
	}
	return nil
}
