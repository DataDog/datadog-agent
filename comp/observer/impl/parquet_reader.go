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
// Uses GetRecordReader instead of ReadTable for proper nested type handling (list<string>).
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

	// Find column indices (case-insensitive, underscore-stripping)
	runIDIdx := findColumnIndex(schema, "runid")
	timeIdx := findColumnIndex(schema, "time")
	metricNameIdx := findColumnIndex(schema, "metricname")
	valueIntIdx := findColumnIndex(schema, "valueint")
	valueFloatIdx := findColumnIndex(schema, "valuefloat")
	tagsIdx := findColumnIndex(schema, "tags")

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

	// Get typed column references
	var runIDCol *array.String
	var metricNameCol *array.String
	var valueIntCol *array.Uint64
	var valueFloatCol *array.Float64
	var tagsCol *array.List
	var timeColRaw arrow.Array

	if runIDIdx >= 0 {
		if c, ok := record.Column(runIDIdx).(*array.String); ok {
			runIDCol = c
		}
	}
	if timeIdx >= 0 {
		timeColRaw = record.Column(timeIdx)
	}
	if metricNameIdx >= 0 {
		if c, ok := record.Column(metricNameIdx).(*array.String); ok {
			metricNameCol = c
		}
	}
	if valueIntIdx >= 0 {
		if c, ok := record.Column(valueIntIdx).(*array.Uint64); ok {
			valueIntCol = c
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
		var valueInt *uint64
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
		if valueIntCol != nil && !valueIntCol.IsNull(i) {
			v := valueIntCol.Value(i)
			valueInt = &v
		}
		if valueFloatCol != nil && !valueFloatCol.IsNull(i) {
			v := valueFloatCol.Value(i)
			valueFloat = &v
		}

		// Extract tags from list column (recorder format: Tags list<string>)
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
			ValueInt:   valueInt,
			ValueFloat: valueFloat,
			Tags:       tags,
		}
	}

	return metrics, nil
}

// normalizeColumnName lowercases and strips underscores for flexible matching.
func normalizeColumnName(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

// findColumnIndex finds the index of a column by name (case-insensitive, underscore-stripping).
func findColumnIndex(schema *arrow.Schema, name string) int {
	normalized := normalizeColumnName(name)
	for i, field := range schema.Fields() {
		if normalizeColumnName(field.Name) == normalized {
			return i
		}
	}
	return -1
}

// getAt safely gets a string from a slice by index.
func getAt(slice []string, idx int) string {
	if idx < len(slice) {
		return slice[idx]
	}
	return ""
}
