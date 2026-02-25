// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
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
// Will return an empty slice if the file is too small (no error).
func readParquetFile(filePath string) ([]FGMMetric, error) {
	// Ensure file is not too small
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("statting file: %w", err)
	}
	if info.Size() < minParquetFileSize {
		return []FGMMetric{}, nil
	}

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

	// Create arrow file reader with proper allocator for nested types
	arrowReadProps := pqarrow.ArrowReadProperties{BatchSize: 1024}
	arrowReader, err := pqarrow.NewFileReader(parquetReader, arrowReadProps, memory.DefaultAllocator)
	if err != nil {
		return nil, fmt.Errorf("creating arrow reader: %w", err)
	}

	// Use GetRecordReader for proper nested type handling (list<string>)
	ctx := context.Background()
	recordReader, err := arrowReader.GetRecordReader(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("getting record reader: %w", err)
	}
	defer recordReader.Release()

	// Extract metrics from record batches
	var allMetrics []FGMMetric
	for recordReader.Next() {
		record := recordReader.Record()
		metrics, err := extractMetricsFromRecord(record)
		if err != nil {
			return nil, fmt.Errorf("extracting metrics: %w", err)
		}
		allMetrics = append(allMetrics, metrics...)
	}

	// EOF is expected, other errors are not
	if err := recordReader.Err(); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("reading records: %w", err)
	}

	return allMetrics, nil
}

// extractMetricsFromRecord extracts FGMMetric structs from an Arrow record batch.
func extractMetricsFromRecord(record arrow.Record) ([]FGMMetric, error) {
	numRows := int(record.NumRows())
	if numRows == 0 {
		return nil, nil
	}

	schema := record.Schema()

	// Find column indices (case-insensitive)
	runIDIdx := findColumnIndexInSchema(schema, "runid")
	timeIdx := findColumnIndexInSchema(schema, "time")
	metricNameIdx := findColumnIndexInSchema(schema, "metricname")
	valueFloatIdx := findColumnIndexInSchema(schema, "valuefloat")
	tagsIdx := findColumnIndexInSchema(schema, "tags")

	// Helper to read int64 values from either Int64 or Timestamp columns
	readTimeValue := func(col arrow.Array, i int) int64 {
		if col.IsNull(i) {
			return 0
		}
		switch c := col.(type) {
		case *array.Int64:
			return c.Value(i)
		case *array.Timestamp:
			return int64(c.Value(i))
		default:
			return 0
		}
	}

	// Get typed column references (safe type switches)
	var runIDCol *array.String
	var metricNameCol *array.String
	var valueFloatCol *array.Float64
	var tagsCol *array.List
	var timeColRaw arrow.Array

	if runIDIdx >= 0 {
		if c, ok := record.Column(runIDIdx).(*array.String); ok {
			runIDCol = c
		}
	}
	if timeIdx >= 0 {
		timeColRaw = record.Column(timeIdx) // Int64 or Timestamp, handled by readTimeValue
	}
	if metricNameIdx >= 0 {
		if c, ok := record.Column(metricNameIdx).(*array.String); ok {
			metricNameCol = c
		}
	}
	if valueFloatIdx >= 0 {
		if c, ok := record.Column(valueFloatIdx).(*array.Float64); ok {
			valueFloatCol = c
		}
	}
	if tagsIdx >= 0 {
		if c, ok := record.Column(tagsIdx).(*array.List); ok {
			tagsCol = c
		}
	}

	// Discover l_* tag columns (FGM format: flat string columns prefixed with "l_")
	type labelCol struct {
		key string
		col *array.String
	}
	var labelCols []labelCol
	for i, field := range schema.Fields() {
		if strings.HasPrefix(field.Name, "l_") {
			if c, ok := record.Column(i).(*array.String); ok {
				labelCols = append(labelCols, labelCol{
					key: strings.TrimPrefix(field.Name, "l_"),
					col: c,
				})
			}
		}
	}

	// Build metrics
	metrics := make([]FGMMetric, numRows)
	for i := 0; i < numRows; i++ {
		var runID, metricName string
		var timestamp int64
		var valueFloat *float64
		tags := make(map[string]string)

		if runIDCol != nil && !runIDCol.IsNull(i) {
			runID = runIDCol.Value(i)
		}
		if timeColRaw != nil {
			timestamp = readTimeValue(timeColRaw, i)
		}
		if metricNameCol != nil && !metricNameCol.IsNull(i) {
			metricName = metricNameCol.Value(i)
		}
		if valueFloatCol != nil && !valueFloatCol.IsNull(i) {
			v := valueFloatCol.Value(i)
			valueFloat = &v
		}

		// Extract tags from list column (recorder format)
		if tagsCol != nil && !tagsCol.IsNull(i) {
			start, end := tagsCol.ValueOffsets(i)
			tagsValues := tagsCol.ListValues().(*array.String)
			for j := start; j < end; j++ {
				if !tagsValues.IsNull(int(j)) {
					tag := tagsValues.Value(int(j))
					parts := strings.SplitN(tag, ":", 2)
					if len(parts) == 2 {
						tags[parts[0]] = parts[1]
					} else if len(parts) == 1 && parts[0] != "" {
						tags[parts[0]] = ""
					}
				}
			}
		}

		// Extract tags from l_* columns (FGM format)
		for _, lc := range labelCols {
			if !lc.col.IsNull(i) {
				if v := lc.col.Value(i); v != "" {
					tags[lc.key] = v
				}
			}
		}

		metrics[i] = FGMMetric{
			RunID:      runID,
			Time:       timestamp,
			MetricName: metricName,
			ValueFloat: valueFloat,
			Tags:       tags,
		}
	}

	return metrics, nil
}

// normalizeColumnName lowercases and strips underscores for flexible matching.
// This handles both PascalCase (MetricName) and snake_case (metric_name) column names.
func normalizeColumnName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// findColumnIndexInSchema finds the index of a column by name.
// Matches are case-insensitive and ignore underscores, so "metric_name",
// "MetricName", and "metricname" all match each other.
func findColumnIndexInSchema(schema *arrow.Schema, name string) int {
	normalized := normalizeColumnName(name)
	for i, field := range schema.Fields() {
		if normalizeColumnName(field.Name) == normalized {
			return i
		}
	}
	return -1
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

// findColumnIndex finds the index of a column by name, returns -1 if not found.
// Uses the same normalization as findColumnIndexInSchema.
func findColumnIndex(schema *arrow.Schema, name string) int {
	return findColumnIndexInSchema(schema, name)
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
		// Handle both String and Binary array types
		listValues := listArr.ListValues()
		if listValues == nil {
			offset += listArr.Len()
			continue
		}

		// Type assert once per chunk
		strArr, isString := listValues.(*array.String)
		binArr, isBinary := listValues.(*array.Binary)

		if !isString && !isBinary {
			// Unknown type, skip this chunk
			offset += listArr.Len()
			continue
		}

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

			if isString {
				for j := start; j < end; j++ {
					if !strArr.IsNull(j) {
						tags = append(tags, strArr.Value(j))
					}
				}
			} else if isBinary {
				for j := start; j < end; j++ {
					if !binArr.IsNull(j) {
						tags = append(tags, string(binArr.Value(j)))
					}
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
