// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

// Parquet reading utilities for the testbench.
// Arrow/parquet deps live only in the testbench's own go.mod — not in the
// main agent module — so this file must stay in internal/qbranch/.

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

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ParquetFormat selects which parquet layout to read.
// Use FormatAuto (empty string) to detect automatically by presence of contexts.parquet.
type ParquetFormat string

const (
	FormatAuto ParquetFormat = ""   // detect: v2 if contexts.parquet present, else v1
	FormatV1   ParquetFormat = "v1" // observer-metrics-*.parquet / observer-logs-*.parquet (inline tags)
	FormatV2   ParquetFormat = "v2" // contexts.parquet + metrics-*.parquet / logs-*.parquet
)

// detectParquetFormat returns FormatV2 if contexts.parquet exists in dir, else FormatV1.
func detectParquetFormat(dir string) ParquetFormat {
	if _, err := os.Stat(filepath.Join(dir, "contexts.parquet")); err == nil {
		return FormatV2
	}
	return FormatV1
}

// ---- Metric reader ----

type fgmMetric struct {
	RunID      string
	Time       int64
	MetricName string
	ValueInt   *uint64
	ValueFloat *float64
	Tags       map[string]string
	Dropped    bool
}

type parquetMetricReader struct {
	metrics []fgmMetric
	index   int
}

func newParquetMetricReader(dirPath string) (*parquetMetricReader, error) {
	files, err := findMetricParquetFiles(dirPath)
	if err != nil {
		return nil, fmt.Errorf("finding parquet files: %w", err)
	}
	if len(files) == 0 {
		return &parquetMetricReader{}, nil
	}
	var all []fgmMetric
	for _, f := range files {
		metrics, err := readMetricParquetFile(f)
		if err != nil {
			fmt.Printf("[parquet-reader] Skipping %s: %v\n", f, err)
			continue
		}
		all = append(all, metrics...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].Time < all[j].Time })
	return &parquetMetricReader{metrics: all}, nil
}

func (r *parquetMetricReader) next() *fgmMetric {
	if r.index >= len(r.metrics) {
		return nil
	}
	m := &r.metrics[r.index]
	r.index++
	return m
}

func (r *parquetMetricReader) len() int { return len(r.metrics) }

const minParquetFileSize = 20

func findMetricParquetFiles(dirPath string) ([]string, error) {
	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		name := info.Name()
		if !info.IsDir() && strings.HasPrefix(name, "observer-metrics-") && strings.HasSuffix(name, ".parquet") {
			if info.Size() < minParquetFileSize {
				fmt.Printf("[parquet-reader] Skipping %s: file too small (%d bytes)\n", path, info.Size())
				return nil
			}
			files = append(files, path)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func readMetricParquetFile(filePath string) ([]fgmMetric, error) {
	var all []fgmMetric
	err := streamMetricParquetFileV1(filePath, func(metric fgmMetric) error {
		all = append(all, metric)
		return nil
	})
	return all, err
}

func streamMetricParquetFileV1(filePath string, fn func(fgmMetric) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("statting file: %w", err)
	}
	if info.Size() < minParquetFileSize {
		return fmt.Errorf("file is too small to be parquet (%d bytes)", info.Size())
	}
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		return fmt.Errorf("creating parquet reader: %w", err)
	}
	defer pf.Close()

	arrowReader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
	if err != nil {
		return fmt.Errorf("creating arrow reader: %w", err)
	}

	ctx := context.Background()
	rr, err := arrowReader.GetRecordReader(ctx, nil, nil)
	if err != nil {
		return fmt.Errorf("getting record reader: %w", err)
	}
	defer rr.Release()

	for rr.Next() {
		rec := rr.Record()
		metrics, err := extractMetricsFromRecord(rec)
		if err != nil {
			rec.Release()
			return fmt.Errorf("extracting metrics: %w", err)
		}
		for _, metric := range metrics {
			if err := fn(metric); err != nil {
				rec.Release()
				return err
			}
		}
		rec.Release()
	}
	if err := rr.Err(); err != nil && err.Error() != "EOF" {
		return fmt.Errorf("reading records: %w", err)
	}
	return nil
}

func extractMetricsFromRecord(rec arrow.Record) ([]fgmMetric, error) {
	n := int(rec.NumRows())
	if n == 0 {
		return nil, nil
	}
	schema := rec.Schema()

	runIDIdx := findCol(schema, "runid")
	timeIdx := findCol(schema, "time")
	metricNameIdx := findCol(schema, "metricname")
	valueFloatIdx := findCol(schema, "valuefloat")
	tagsIdx := findCol(schema, "tags")
	droppedIdx := findCol(schema, "dropped")

	readTime := func(col arrow.Array, i int) int64 {
		if col == nil || col.IsNull(i) {
			return 0
		}
		switch c := col.(type) {
		case *array.Int64:
			return c.Value(i)
		case *array.Timestamp:
			return int64(c.Value(i))
		}
		return 0
	}

	var runIDCol, metricNameCol *array.String
	var valueFloatCol *array.Float64
	var tagsCol *array.List
	var droppedCol *array.Boolean
	var timeColRaw arrow.Array

	if runIDIdx >= 0 {
		if c, ok := rec.Column(runIDIdx).(*array.String); ok {
			runIDCol = c
		}
	}
	if timeIdx >= 0 {
		timeColRaw = rec.Column(timeIdx)
	}
	if metricNameIdx >= 0 {
		if c, ok := rec.Column(metricNameIdx).(*array.String); ok {
			metricNameCol = c
		}
	}
	if valueFloatIdx >= 0 {
		if c, ok := rec.Column(valueFloatIdx).(*array.Float64); ok {
			valueFloatCol = c
		}
	}
	if tagsIdx >= 0 {
		if c, ok := rec.Column(tagsIdx).(*array.List); ok {
			tagsCol = c
		}
	}
	if droppedIdx >= 0 {
		if c, ok := rec.Column(droppedIdx).(*array.Boolean); ok {
			droppedCol = c
		}
	}

	// l_* label columns (FGM format)
	type labelCol struct {
		key string
		col *array.String
	}
	var labelCols []labelCol
	for i, field := range schema.Fields() {
		if strings.HasPrefix(field.Name, "l_") {
			if c, ok := rec.Column(i).(*array.String); ok {
				labelCols = append(labelCols, labelCol{strings.TrimPrefix(field.Name, "l_"), c})
			}
		}
	}

	metrics := make([]fgmMetric, n)
	for i := 0; i < n; i++ {
		tags := make(map[string]string)
		var runID, metricName string
		var valueFloat *float64
		if runIDCol != nil && !runIDCol.IsNull(i) {
			runID = runIDCol.Value(i)
		}
		ts := readTime(timeColRaw, i)
		if metricNameCol != nil && !metricNameCol.IsNull(i) {
			metricName = metricNameCol.Value(i)
		}
		if valueFloatCol != nil && !valueFloatCol.IsNull(i) {
			v := valueFloatCol.Value(i)
			valueFloat = &v
		}
		if tagsCol != nil && !tagsCol.IsNull(i) {
			start, end := tagsCol.ValueOffsets(i)
			sv := tagsCol.ListValues().(*array.String)
			for j := start; j < end; j++ {
				if !sv.IsNull(int(j)) {
					tag := sv.Value(int(j))
					parts := strings.SplitN(tag, ":", 2)
					if len(parts) == 2 {
						tags[parts[0]] = parts[1]
					} else if len(parts) == 1 && parts[0] != "" {
						tags[parts[0]] = ""
					}
				}
			}
		}
		for _, lc := range labelCols {
			if !lc.col.IsNull(i) {
				if v := lc.col.Value(i); v != "" {
					tags[lc.key] = v
				}
			}
		}
		var dropped bool
		if droppedCol != nil && !droppedCol.IsNull(i) {
			dropped = droppedCol.Value(i)
		}
		metrics[i] = fgmMetric{
			RunID:      runID,
			Time:       ts,
			MetricName: metricName,
			ValueFloat: valueFloat,
			Tags:       tags,
			Dropped:    dropped,
		}
	}
	return metrics, nil
}

func normalizeCol(name string) string {
	return strings.ToLower(strings.ReplaceAll(name, "_", ""))
}

func findCol(schema *arrow.Schema, name string) int {
	n := normalizeCol(name)
	for i, f := range schema.Fields() {
		if normalizeCol(f.Name) == n {
			return i
		}
	}
	return -1
}

// readAllMetrics reads all FGM parquet files from dir and returns MetricData records.
func readAllMetrics(dir string) ([]recorderdef.MetricData, error) {
	reader, err := newParquetMetricReader(dir)
	if err != nil {
		return nil, err
	}
	pkglog.Infof("ReadAllMetrics: loading %d metrics from %s", reader.len(), dir)
	out := make([]recorderdef.MetricData, 0, reader.len())
	for {
		m := reader.next()
		if m == nil {
			break
		}
		out = append(out, metricDataFromFGM(*m))
	}
	pkglog.Infof("ReadAllMetrics: loaded %d metrics", len(out))
	return out, nil
}

func metricDataFromFGM(metric fgmMetric) recorderdef.MetricData {
	var value float64
	if metric.ValueFloat != nil {
		value = *metric.ValueFloat
	} else if metric.ValueInt != nil {
		value = float64(*metric.ValueInt)
	}
	tags := make([]string, 0, len(metric.Tags))
	for key, tagValue := range metric.Tags {
		if tagValue != "" {
			tags = append(tags, key+":"+tagValue)
		} else {
			tags = append(tags, key)
		}
	}
	return recorderdef.MetricData{
		Source:    metric.RunID,
		Name:      metric.MetricName,
		Value:     value,
		Timestamp: metric.Time / 1000,
		Tags:      tags,
		Dropped:   metric.Dropped,
	}
}

// streamOrderedMetrics reads metric parquet files in filename and row order
// without retaining the full dataset. Equal timestamps are allowed.
func streamOrderedMetrics(dir string, format ParquetFormat, fn func(recorderdef.MetricData) error) (int, error) {
	return streamOrderedMetricsWithContexts(dir, format, nil, fn)
}

func streamOrderedMetricsWithContexts(
	dir string,
	format ParquetFormat,
	v2Contexts map[uint64]contextEntryV2,
	fn func(recorderdef.MetricData) error,
) (int, error) {
	var (
		count        int
		previousTime int64
		havePrevious bool
	)

	consume := func(filePath string, metric recorderdef.MetricData) error {
		if havePrevious && metric.Timestamp < previousTime {
			return fmt.Errorf(
				"metric timestamps are not globally ordered: %s contains %d after %d",
				filepath.Base(filePath), metric.Timestamp, previousTime,
			)
		}
		if err := fn(metric); err != nil {
			return err
		}
		previousTime = metric.Timestamp
		havePrevious = true
		count++
		return nil
	}

	var err error
	if format == FormatV2 {
		if v2Contexts == nil {
			err = streamAllMetricsV2(dir, consume)
		} else {
			err = streamAllMetricsV2WithContexts(dir, v2Contexts, consume)
		}
	} else {
		err = streamAllMetricsV1(dir, consume)
	}
	if err != nil {
		return count, err
	}
	return count, nil
}

func streamAllMetricsV1(dir string, fn func(string, recorderdef.MetricData) error) error {
	files, err := findMetricParquetFiles(dir)
	if err != nil {
		return fmt.Errorf("listing v1 metric parquet files: %w", err)
	}
	for _, filePath := range files {
		if err := streamMetricParquetFileV1(filePath, func(metric fgmMetric) error {
			return fn(filePath, metricDataFromFGM(metric))
		}); err != nil {
			return fmt.Errorf("streaming %s: %w", filePath, err)
		}
	}
	return nil
}

// ---- Log reader ----

// streamOrderedLogs reads log parquet files in filename and row order without
// retaining the full dataset. The caller must provide parquet files whose rows
// are globally ordered by timestamp; equal timestamps are allowed.
func streamOrderedLogs(dir string, format ParquetFormat, fn func(recorderdef.LogData) error) (int, error) {
	return streamOrderedLogsWithContexts(dir, format, nil, fn)
}

func streamOrderedLogsWithContexts(
	dir string,
	format ParquetFormat,
	v2Contexts map[uint64]contextEntryV2,
	fn func(recorderdef.LogData) error,
) (int, error) {
	var (
		count        int
		previousTime int64
		havePrevious bool
	)

	consume := func(filePath string, entry recorderdef.LogData) error {
		if havePrevious && entry.TimestampMs < previousTime {
			return fmt.Errorf(
				"log timestamps are not globally ordered: %s contains %d after %d",
				filepath.Base(filePath), entry.TimestampMs, previousTime,
			)
		}
		if err := fn(entry); err != nil {
			return err
		}
		previousTime = entry.TimestampMs
		havePrevious = true
		count++
		return nil
	}

	var err error
	if format == FormatV2 {
		if v2Contexts == nil {
			err = streamAllLogsV2(dir, consume)
		} else {
			err = streamAllLogsV2WithContexts(dir, v2Contexts, consume)
		}
	} else {
		err = streamAllLogsV1(dir, consume)
	}
	if err != nil {
		return count, err
	}
	return count, nil
}

func findLogParquetFilesV1(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "observer-logs-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func streamAllLogsV1(dir string, fn func(string, recorderdef.LogData) error) error {
	files, err := findLogParquetFilesV1(dir)
	if err != nil {
		return fmt.Errorf("listing v1 log parquet files: %w", err)
	}
	for _, filePath := range files {
		if err := streamLogParquetFileV1(filePath, func(entry recorderdef.LogData) error {
			return fn(filePath, entry)
		}); err != nil {
			return fmt.Errorf("streaming %s: %w", filePath, err)
		}
	}
	return nil
}

func streamLogParquetFileV1(filePath string, fn func(recorderdef.LogData) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("statting file: %w", err)
	}
	if info.Size() < minParquetFileSize {
		return fmt.Errorf("file is too small to be parquet (%d bytes)", info.Size())
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		return fmt.Errorf("creating parquet reader: %w", err)
	}
	defer pf.Close()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
	if err != nil {
		return fmt.Errorf("creating arrow reader: %w", err)
	}

	rr, err := reader.GetRecordReader(context.Background(), nil, nil)
	if err != nil {
		return fmt.Errorf("getting record reader: %w", err)
	}
	defer rr.Release()

	for rr.Next() {
		rec := rr.Record()
		if err := streamLogsFromRecordV1(rec, fn); err != nil {
			rec.Release()
			return err
		}
		rec.Release()
	}
	if err := rr.Err(); err != nil && err.Error() != "EOF" {
		return fmt.Errorf("reading records: %w", err)
	}
	return nil
}

func streamLogsFromRecordV1(rec arrow.Record, fn func(recorderdef.LogData) error) error {
	schema := rec.Schema()
	timeIdx := findCol(schema, "time")
	if timeIdx < 0 {
		return fmt.Errorf("missing required Time column")
	}
	timeCol, ok := rec.Column(timeIdx).(*array.Int64)
	if !ok {
		return fmt.Errorf("Time column has type %T, expected int64", rec.Column(timeIdx))
	}

	getString := func(name string) (*array.String, error) {
		idx := findCol(schema, name)
		if idx < 0 {
			return nil, nil
		}
		col, ok := rec.Column(idx).(*array.String)
		if !ok {
			return nil, fmt.Errorf("%s column has type %T, expected string", name, rec.Column(idx))
		}
		return col, nil
	}

	runIDCol, err := getString("runid")
	if err != nil {
		return err
	}
	statusCol, err := getString("status")
	if err != nil {
		return err
	}
	hostnameCol, err := getString("hostname")
	if err != nil {
		return err
	}

	var contentCol *array.Binary
	if idx := findCol(schema, "content"); idx >= 0 {
		contentCol, ok = rec.Column(idx).(*array.Binary)
		if !ok {
			return fmt.Errorf("Content column has type %T, expected binary", rec.Column(idx))
		}
	}

	var tagsCol *array.List
	if idx := findCol(schema, "tags"); idx >= 0 {
		tagsCol, ok = rec.Column(idx).(*array.List)
		if !ok {
			return fmt.Errorf("Tags column has type %T, expected list", rec.Column(idx))
		}
		if _, ok := tagsCol.ListValues().(*array.String); !ok {
			return fmt.Errorf("Tags values have type %T, expected string", tagsCol.ListValues())
		}
	}

	for i := 0; i < int(rec.NumRows()); i++ {
		if timeCol.IsNull(i) {
			return fmt.Errorf("Time is null at row %d", i)
		}
		entry := recorderdef.LogData{TimestampMs: timeCol.Value(i)}
		if runIDCol != nil && !runIDCol.IsNull(i) {
			entry.Source = runIDCol.Value(i)
		}
		if contentCol != nil && !contentCol.IsNull(i) {
			entry.Content = append([]byte(nil), contentCol.Value(i)...)
		}
		if statusCol != nil && !statusCol.IsNull(i) {
			entry.Status = statusCol.Value(i)
		}
		if hostnameCol != nil && !hostnameCol.IsNull(i) {
			entry.Hostname = hostnameCol.Value(i)
		}
		if tagsCol != nil {
			entry.Tags = readStringList(tagsCol, i)
		}
		if err := fn(entry); err != nil {
			return err
		}
	}
	return nil
}

// readAllLogs reads all log parquet files from dir and returns LogData records.
func readAllLogs(dir string) ([]recorderdef.LogData, error) {
	files, err := findLogParquetFilesV1(dir)
	if err != nil {
		return nil, err
	}

	var logs []recorderdef.LogData
	for _, f := range files {
		readLogParquetFile(f, &logs)
	}
	sort.SliceStable(logs, func(i, j int) bool { return logs[i].TimestampMs < logs[j].TimestampMs })
	pkglog.Infof("ReadAllLogs: loaded %d logs from %s", len(logs), dir)
	return logs, nil
}

func readLogParquetFile(filePath string, logs *[]recorderdef.LogData) {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open log parquet file %s: %v", filePath, err)
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

	// Iterate record batches so each batch's column arrays are single-chunk
	// and Value(i) is always in-bounds regardless of parquet row-group count.
	tr := array.NewTableReader(table, 1024)
	defer tr.Release()
	for tr.Next() {
		rec := tr.Record()
		n := int(rec.NumRows())

		getStr := func(name string) *array.String {
			if idx, ok := colIdx[name]; ok {
				c, _ := rec.Column(idx).(*array.String)
				return c
			}
			return nil
		}
		getI64 := func(name string) *array.Int64 {
			if idx, ok := colIdx[name]; ok {
				c, _ := rec.Column(idx).(*array.Int64)
				return c
			}
			return nil
		}
		getBin := func(name string) *array.Binary {
			if idx, ok := colIdx[name]; ok {
				c, _ := rec.Column(idx).(*array.Binary)
				return c
			}
			return nil
		}
		getLst := func(name string) *array.List {
			if idx, ok := colIdx[name]; ok {
				c, _ := rec.Column(idx).(*array.List)
				return c
			}
			return nil
		}

		runIDCol := getStr("RunID")
		timeCol := getI64("Time")
		contentCol := getBin("Content")
		statusCol := getStr("Status")
		hostnameCol := getStr("Hostname")
		tagsCol := getLst("Tags")

		for i := 0; i < n; i++ {
			entry := recorderdef.LogData{}
			if runIDCol != nil {
				entry.Source = runIDCol.Value(i)
			}
			if timeCol != nil {
				entry.TimestampMs = timeCol.Value(i)
			}
			if contentCol != nil && !contentCol.IsNull(i) {
				data := contentCol.Value(i)
				entry.Content = make([]byte, len(data))
				copy(entry.Content, data)
			}
			if statusCol != nil {
				entry.Status = statusCol.Value(i)
			}
			if hostnameCol != nil {
				entry.Hostname = hostnameCol.Value(i)
			}
			if tagsCol != nil {
				entry.Tags = readStringList(tagsCol, i)
			}
			*logs = append(*logs, entry)
		}
	}
}

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
