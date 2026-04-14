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
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
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

// parquetMetricReader reads FGM parquet files and provides metrics in chronological order.
type parquetMetricReader struct {
	metrics []FGMMetric
	index   int
}

// newParquetReader creates a new parquet reader from a directory containing parquet files.
// It supports two layouts:
//   - New layout (contexts.parquet present): metrics-*.parquet files reference a
//     shared contexts.parquet for deduplication of metric name + tags.
//   - Legacy FGM layout: all context is inline in each parquet file.
func newParquetReader(dirPath string) (*parquetMetricReader, error) {
	// Detect new layout by the presence of contexts.parquet.
	if _, err := os.Stat(filepath.Join(dirPath, "contexts.parquet")); err == nil {
		allMetrics, err := readNewFormatMetricsDir(dirPath)
		if err != nil {
			return nil, fmt.Errorf("reading new-format metrics: %w", err)
		}
		sort.Slice(allMetrics, func(i, j int) bool {
			return allMetrics[i].Time < allMetrics[j].Time
		})
		return &parquetMetricReader{metrics: allMetrics, index: 0}, nil
	}

	// Legacy FGM layout: find all parquet files in the directory.
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

	return &parquetMetricReader{
		metrics: allMetrics,
		index:   0,
	}, nil
}

// contextEntry holds the metric context read from contexts.parquet.
type contextEntry struct {
	Name string
	Tags []string // pre-built "key:value" tag strings
}

// readContextsFile reads contexts.parquet and returns a map from context_key to contextEntry.
func readContextsFile(filePath string) (map[uint64]contextEntry, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening contexts file: %w", err)
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}
	defer pf.Close()

	arrowReader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 8192}, memory.DefaultAllocator)
	if err != nil {
		return nil, fmt.Errorf("creating arrow reader: %w", err)
	}

	ctx := context.Background()
	recordReader, err := arrowReader.GetRecordReader(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("getting record reader: %w", err)
	}
	defer recordReader.Release()

	contexts := make(map[uint64]contextEntry)

	for recordReader.Next() {
		record := recordReader.Record()
		schema := record.Schema()

		ctxKeyIdx := findColumnIndexInSchema(schema, "context_key")
		nameIdx := findColumnIndexInSchema(schema, "name")
		tagHostIdx := findColumnIndexInSchema(schema, "tag_host")
		tagDeviceIdx := findColumnIndexInSchema(schema, "tag_device")
		tagSourceIdx := findColumnIndexInSchema(schema, "tag_source")
		tagServiceIdx := findColumnIndexInSchema(schema, "tag_service")
		tagEnvIdx := findColumnIndexInSchema(schema, "tag_env")
		tagVersionIdx := findColumnIndexInSchema(schema, "tag_version")
		tagTeamIdx := findColumnIndexInSchema(schema, "tag_team")
		tagsIdx := findColumnIndexInSchema(schema, "tags")

		if ctxKeyIdx < 0 || nameIdx < 0 {
			return nil, fmt.Errorf("contexts.parquet missing required columns (context_key, name)")
		}

		ctxKeyCol, _ := record.Column(ctxKeyIdx).(*array.Uint64)
		nameCol, _ := record.Column(nameIdx).(*array.String)

		getStrCol := func(idx int) *array.String {
			if idx < 0 {
				return nil
			}
			col, _ := record.Column(idx).(*array.String)
			return col
		}
		tagHostCol := getStrCol(tagHostIdx)
		tagDeviceCol := getStrCol(tagDeviceIdx)
		tagSourceCol := getStrCol(tagSourceIdx)
		tagServiceCol := getStrCol(tagServiceIdx)
		tagEnvCol := getStrCol(tagEnvIdx)
		tagVersionCol := getStrCol(tagVersionIdx)
		tagTeamCol := getStrCol(tagTeamIdx)

		var tagsMapCol *array.Map
		if tagsIdx >= 0 {
			tagsMapCol, _ = record.Column(tagsIdx).(*array.Map)
		}

		numRows := int(record.NumRows())
		for i := 0; i < numRows; i++ {
			if ctxKeyCol == nil || ctxKeyCol.IsNull(i) {
				continue
			}
			key := ctxKeyCol.Value(i)

			var name string
			if nameCol != nil && !nameCol.IsNull(i) {
				name = nameCol.Value(i)
			}

			tags := make([]string, 0, 8)
			addFixedTag := func(col *array.String, tagKey string) {
				if col == nil || col.IsNull(i) {
					return
				}
				if v := col.Value(i); v != "" {
					tags = append(tags, tagKey+":"+v)
				}
			}
			addFixedTag(tagHostCol, "host")
			addFixedTag(tagDeviceCol, "device")
			addFixedTag(tagSourceCol, "source")
			addFixedTag(tagServiceCol, "service")
			addFixedTag(tagEnvCol, "env")
			addFixedTag(tagVersionCol, "version")
			addFixedTag(tagTeamCol, "team")

			// Extract tags from the map<string,string> column.
			if tagsMapCol != nil && !tagsMapCol.IsNull(i) {
				start, end := tagsMapCol.ValueOffsets(i)
				mapKeys, _ := tagsMapCol.Keys().(*array.String)
				mapItems, _ := tagsMapCol.Items().(*array.String)
				for j := int(start); j < int(end); j++ {
					if mapKeys == nil || mapKeys.IsNull(j) {
						continue
					}
					k := mapKeys.Value(j)
					if k == "" {
						continue
					}
					if mapItems != nil && !mapItems.IsNull(j) {
						if v := mapItems.Value(j); v != "" {
							tags = append(tags, k+":"+v)
							continue
						}
					}
					tags = append(tags, k)
				}
			}

			contexts[key] = contextEntry{Name: name, Tags: tags}
		}
	}

	if err := recordReader.Err(); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("reading contexts records: %w", err)
	}

	return contexts, nil
}

// readNewFormatMetricsDir reads the new context-deduped layout from dir:
// it loads contexts.parquet once, then joins each metrics-*.parquet against it.
func readNewFormatMetricsDir(dir string) ([]FGMMetric, error) {
	contexts, err := readContextsFile(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return nil, fmt.Errorf("loading contexts: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory: %w", err)
	}

	var allMetrics []FGMMetric
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "metrics-") || !strings.HasSuffix(name, ".parquet") {
			continue
		}
		filePath := filepath.Join(dir, name)
		metrics, err := readNewFormatMetricsFile(filePath, contexts)
		if err != nil {
			fmt.Printf("[parquet-reader] Skipping %s: %v\n", filePath, err)
			continue
		}
		allMetrics = append(allMetrics, metrics...)
	}

	return allMetrics, nil
}

// readNewFormatMetricsFile reads a single metrics-*.parquet file and joins rows with contexts.
// Schema: context_key uint64, value double, timestamp_ns int64, sample_rate double, source dict<string>.
//
// Uses file.ColumnChunkReader.ReadBatch directly (bypassing pqarrow) to avoid the
// arrow-go v18 bug where pqarrow calls GetDictionaryPage() out-of-band without advancing
// the main stream, causing a second configureDict() call when Next() also encounters the
// dictionary page — resulting in "column chunk cannot have more than one dictionary".
// The file package handles dictionary decoding transparently without this issue.
func readNewFormatMetricsFile(filePath string, contexts map[uint64]contextEntry) ([]FGMMetric, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf("statting file: %w", err)
	}
	if info.Size() < minParquetFileSize {
		return nil, nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		return nil, fmt.Errorf("creating parquet reader: %w", err)
	}
	defer pf.Close()

	s := pf.MetaData().Schema
	ctxKeyIdx := findParquetColIdx(pf, "context_key")
	valueIdx := findParquetColIdx(pf, "value")
	tsIdx := findParquetColIdx(pf, "timestamp_ns")
	sourceIdx := findParquetColIdx(pf, "source")

	if ctxKeyIdx < 0 || valueIdx < 0 || tsIdx < 0 {
		return nil, fmt.Errorf("metrics file missing required columns (context_key, value, timestamp_ns)")
	}

	// MaxDefinitionLevel is 0 for required (non-nullable) columns and 1 for optional ones.
	// A row is null when defLevel < maxDefLevel; required columns are never null.
	ctxKeyMaxDef := s.Column(ctxKeyIdx).MaxDefinitionLevel()
	valueMaxDef := s.Column(valueIdx).MaxDefinitionLevel()
	tsMaxDef := s.Column(tsIdx).MaxDefinitionLevel()
	var srcMaxDef int16
	if sourceIdx >= 0 {
		srcMaxDef = s.Column(sourceIdx).MaxDefinitionLevel()
	}

	var metrics []FGMMetric

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		rgReader := pf.RowGroup(rg)
		numRows := int(rgReader.NumRows())
		if numRows == 0 {
			continue
		}

		// Read context_key (stored as int64 representing uint64).
		ctxKeyBuf := make([]int64, numRows)
		ctxKeyDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(ctxKeyIdx)
			if colErr != nil {
				return nil, fmt.Errorf("rg %d: opening context_key: %w", rg, colErr)
			}
			r, ok := col.(*file.Int64ColumnChunkReader)
			if !ok {
				return nil, fmt.Errorf("rg %d: context_key is not int64", rg)
			}
			if _, _, colErr = r.ReadBatch(int64(numRows), ctxKeyBuf, ctxKeyDef, nil); colErr != nil {
				return nil, fmt.Errorf("rg %d: reading context_key: %w", rg, colErr)
			}
		}

		// Read value (float64).
		valueBuf := make([]float64, numRows)
		valueDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(valueIdx)
			if colErr != nil {
				return nil, fmt.Errorf("rg %d: opening value: %w", rg, colErr)
			}
			r, ok := col.(*file.Float64ColumnChunkReader)
			if !ok {
				return nil, fmt.Errorf("rg %d: value is not float64", rg)
			}
			if _, _, colErr = r.ReadBatch(int64(numRows), valueBuf, valueDef, nil); colErr != nil {
				return nil, fmt.Errorf("rg %d: reading value: %w", rg, colErr)
			}
		}

		// Read timestamp_ns (int64).
		tsBuf := make([]int64, numRows)
		tsDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(tsIdx)
			if colErr != nil {
				return nil, fmt.Errorf("rg %d: opening timestamp_ns: %w", rg, colErr)
			}
			r, ok := col.(*file.Int64ColumnChunkReader)
			if !ok {
				return nil, fmt.Errorf("rg %d: timestamp_ns is not int64", rg)
			}
			if _, _, colErr = r.ReadBatch(int64(numRows), tsBuf, tsDef, nil); colErr != nil {
				return nil, fmt.Errorf("rg %d: reading timestamp_ns: %w", rg, colErr)
			}
		}

		// Read source (optional, dict-encoded string — dictionary decoding is transparent).
		var srcBuf []parquet.ByteArray
		var srcDef []int16
		if sourceIdx >= 0 {
			srcBuf = make([]parquet.ByteArray, numRows)
			srcDef = make([]int16, numRows)
			col, colErr := rgReader.Column(sourceIdx)
			if colErr == nil {
				if r, ok := col.(*file.ByteArrayColumnChunkReader); ok {
					if _, _, colErr = r.ReadBatch(int64(numRows), srcBuf, srcDef, nil); colErr != nil {
						srcBuf, srcDef = nil, nil
					}
				} else {
					srcBuf, srcDef = nil, nil
				}
			} else {
				srcBuf, srcDef = nil, nil
			}
		}

		// Build metrics. ReadBatch compacts non-null values into the front of each buffer;
		// defLevels indicate which rows are non-null. We maintain per-column value indices
		// (vi) to iterate the compacted values in sync with the row index.
		ctxVI, valVI, tsVI, srcVI := 0, 0, 0, 0
		for i := 0; i < numRows; i++ {
			ctxNull := ctxKeyDef[i] < ctxKeyMaxDef
			var ctxKey uint64
			if !ctxNull {
				ctxKey = uint64(ctxKeyBuf[ctxVI])
				ctxVI++
			}

			var val float64
			if valueDef[i] >= valueMaxDef {
				val = valueBuf[valVI]
				valVI++
			}

			var ts int64
			if tsDef[i] >= tsMaxDef {
				ts = tsBuf[tsVI]
				tsVI++
			}

			var source string
			if srcBuf != nil && srcDef[i] >= srcMaxDef {
				source = string(srcBuf[srcVI])
				srcVI++
			}

			if ctxNull {
				continue
			}
			entry, ok := contexts[ctxKey]
			if !ok {
				continue
			}

			v := val
			metrics = append(metrics, FGMMetric{
				RunID:      source,
				Time:       ts / 1_000_000,
				MetricName: entry.Name,
				ValueFloat: &v,
				Tags:       tagsMapFromSlice(entry.Tags),
			})
		}
	}

	return metrics, nil
}

// tagsMapFromSlice converts a slice of "key:value" tag strings to the map[string]string
// format expected by FGMMetric.Tags.
func tagsMapFromSlice(tags []string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	m := make(map[string]string, len(tags))
	for _, t := range tags {
		parts := strings.SplitN(t, ":", 2)
		if len(parts) == 2 {
			m[parts[0]] = parts[1]
		} else {
			m[parts[0]] = ""
		}
	}
	return m
}

// Next returns the next metric, or nil if no more metrics.
func (r *parquetMetricReader) Next() *FGMMetric {
	if r.index >= len(r.metrics) {
		return nil
	}
	metric := &r.metrics[r.index]
	r.index++
	return metric
}

// Reset resets the reader to the beginning.
func (r *parquetMetricReader) Reset() {
	r.index = 0
}

// Len returns the total number of metrics.
func (r *parquetMetricReader) Len() int {
	return len(r.metrics)
}

// StartTime returns the timestamp of the first metric in milliseconds.
func (r *parquetMetricReader) StartTime() int64 {
	if len(r.metrics) == 0 {
		return 0
	}
	return r.metrics[0].Time
}

// EndTime returns the timestamp of the last metric in milliseconds.
func (r *parquetMetricReader) EndTime() int64 {
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

// findParquetColIdx finds the index of a column in the parquet file schema by name.
// Uses the same normalization as findColumnIndexInSchema (case-insensitive, underscore-ignored).
func findParquetColIdx(pf *file.Reader, name string) int {
	s := pf.MetaData().Schema
	normalized := normalizeColumnName(name)
	for i := 0; i < s.NumColumns(); i++ {
		if normalizeColumnName(s.Column(i).Name()) == normalized {
			return i
		}
	}
	return -1
}

// ForEachNewFormatMetric streams metrics from a new-format parquet directory (one with
// contexts.parquet), calling fn for each metric. Only one row group worth of column
// buffers is live at a time, so peak memory is O(row_group_size) rather than O(total_metrics).
// Tags are passed as the pre-built []string slice from the context map — callers must not mutate them.
func ForEachNewFormatMetric(dir string, fn func(recorderdef.MetricData) error) error {
	contexts, err := readContextsFile(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return fmt.Errorf("reading contexts: %w", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "metrics-*.parquet"))
	if err != nil {
		return fmt.Errorf("listing metrics files: %w", err)
	}
	sort.Strings(files)

	for _, f := range files {
		if err := forEachNewFormatMetricFile(f, contexts, fn); err != nil {
			return err
		}
	}
	return nil
}

// forEachNewFormatMetricFile streams metrics from a single metrics-*.parquet file.
// Reads one row group at a time; buffers are released before the next row group starts.
func forEachNewFormatMetricFile(filePath string, contexts map[uint64]contextEntry, fn func(recorderdef.MetricData) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return nil
	}
	if info.Size() < minParquetFileSize {
		return nil
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening %s: %w", filePath, err)
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		return fmt.Errorf("creating parquet reader for %s: %w", filePath, err)
	}
	defer pf.Close()

	s := pf.MetaData().Schema
	ctxKeyIdx := findParquetColIdx(pf, "context_key")
	valueIdx := findParquetColIdx(pf, "value")
	tsIdx := findParquetColIdx(pf, "timestamp_ns")
	sourceIdx := findParquetColIdx(pf, "source")

	if ctxKeyIdx < 0 || valueIdx < 0 || tsIdx < 0 {
		return nil // not a metrics file we understand
	}

	ctxKeyMaxDef := s.Column(ctxKeyIdx).MaxDefinitionLevel()
	valueMaxDef := s.Column(valueIdx).MaxDefinitionLevel()
	tsMaxDef := s.Column(tsIdx).MaxDefinitionLevel()
	var srcMaxDef int16
	if sourceIdx >= 0 {
		srcMaxDef = s.Column(sourceIdx).MaxDefinitionLevel()
	}

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		if err := streamRowGroup(pf.RowGroup(rg), ctxKeyIdx, valueIdx, tsIdx, sourceIdx,
			ctxKeyMaxDef, valueMaxDef, tsMaxDef, srcMaxDef, contexts, fn); err != nil {
			return err
		}
	}
	return nil
}

// streamRowGroup reads one row group's columns and calls fn for each matched metric.
// All column buffers are stack-scoped to this call and released when it returns.
func streamRowGroup(
	rgReader *file.RowGroupReader,
	ctxKeyIdx, valueIdx, tsIdx, sourceIdx int,
	ctxKeyMaxDef, valueMaxDef, tsMaxDef, srcMaxDef int16,
	contexts map[uint64]contextEntry,
	fn func(recorderdef.MetricData) error,
) error {
	numRows := int(rgReader.NumRows())
	if numRows == 0 {
		return nil
	}

	ctxKeyBuf := make([]int64, numRows)
	ctxKeyDef := make([]int16, numRows)
	{
		col, err := rgReader.Column(ctxKeyIdx)
		if err != nil {
			return fmt.Errorf("opening context_key: %w", err)
		}
		r, ok := col.(*file.Int64ColumnChunkReader)
		if !ok {
			return fmt.Errorf("context_key is not int64")
		}
		if _, _, err = r.ReadBatch(int64(numRows), ctxKeyBuf, ctxKeyDef, nil); err != nil {
			return fmt.Errorf("reading context_key: %w", err)
		}
	}

	valueBuf := make([]float64, numRows)
	valueDef := make([]int16, numRows)
	{
		col, err := rgReader.Column(valueIdx)
		if err != nil {
			return fmt.Errorf("opening value: %w", err)
		}
		r, ok := col.(*file.Float64ColumnChunkReader)
		if !ok {
			return fmt.Errorf("value is not float64")
		}
		if _, _, err = r.ReadBatch(int64(numRows), valueBuf, valueDef, nil); err != nil {
			return fmt.Errorf("reading value: %w", err)
		}
	}

	tsBuf := make([]int64, numRows)
	tsDef := make([]int16, numRows)
	{
		col, err := rgReader.Column(tsIdx)
		if err != nil {
			return fmt.Errorf("opening timestamp_ns: %w", err)
		}
		r, ok := col.(*file.Int64ColumnChunkReader)
		if !ok {
			return fmt.Errorf("timestamp_ns is not int64")
		}
		if _, _, err = r.ReadBatch(int64(numRows), tsBuf, tsDef, nil); err != nil {
			return fmt.Errorf("reading timestamp_ns: %w", err)
		}
	}

	var srcBuf []parquet.ByteArray
	var srcDef []int16
	if sourceIdx >= 0 {
		srcBuf = make([]parquet.ByteArray, numRows)
		srcDef = make([]int16, numRows)
		col, colErr := rgReader.Column(sourceIdx)
		if colErr == nil {
			if r, ok := col.(*file.ByteArrayColumnChunkReader); ok {
				if _, _, colErr = r.ReadBatch(int64(numRows), srcBuf, srcDef, nil); colErr != nil {
					srcBuf, srcDef = nil, nil
				}
			} else {
				srcBuf, srcDef = nil, nil
			}
		} else {
			srcBuf, srcDef = nil, nil
		}
	}

	ctxVI, valVI, tsVI, srcVI := 0, 0, 0, 0
	for i := 0; i < numRows; i++ {
		ctxNull := ctxKeyDef[i] < ctxKeyMaxDef
		var ctxKey uint64
		if !ctxNull {
			ctxKey = uint64(ctxKeyBuf[ctxVI])
			ctxVI++
		}

		var val float64
		if valueDef[i] >= valueMaxDef {
			val = valueBuf[valVI]
			valVI++
		}

		var tsNs int64
		if tsDef[i] >= tsMaxDef {
			tsNs = tsBuf[tsVI]
			tsVI++
		}

		var source string
		if srcBuf != nil && srcDef[i] >= srcMaxDef {
			source = string(srcBuf[srcVI])
			srcVI++
		}

		if ctxNull {
			continue
		}
		entry, ok := contexts[ctxKey]
		if !ok {
			continue
		}

		if err := fn(recorderdef.MetricData{
			Source:    source,
			Name:      entry.Name,
			Value:     val,
			Timestamp: tsNs / 1_000_000_000,
			Tags:      entry.Tags,
		}); err != nil {
			return err
		}
	}
	return nil
}
