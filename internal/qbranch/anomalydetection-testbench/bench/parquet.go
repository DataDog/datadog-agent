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
	sort.Slice(all, func(i, j int) bool { return all[i].Time < all[j].Time })
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

	arrowReader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
	if err != nil {
		return nil, fmt.Errorf("creating arrow reader: %w", err)
	}

	ctx := context.Background()
	rr, err := arrowReader.GetRecordReader(ctx, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("getting record reader: %w", err)
	}
	defer rr.Release()

	var all []fgmMetric
	for rr.Next() {
		rec := rr.Record()
		metrics, err := extractMetricsFromRecord(rec)
		if err != nil {
			return nil, fmt.Errorf("extracting metrics: %w", err)
		}
		all = append(all, metrics...)
		rec.Release()
	}
	if err := rr.Err(); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("reading records: %w", err)
	}
	return all, nil
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
		var value float64
		if m.ValueFloat != nil {
			value = *m.ValueFloat
		} else if m.ValueInt != nil {
			value = float64(*m.ValueInt)
		}
		tags := make([]string, 0, len(m.Tags))
		for k, v := range m.Tags {
			if v != "" {
				tags = append(tags, k+":"+v)
			} else {
				tags = append(tags, k)
			}
		}
		out = append(out, recorderdef.MetricData{
			Source:    m.RunID,
			Name:      m.MetricName,
			Value:     value,
			Timestamp: m.Time / 1000,
			Tags:      tags,
			Dropped:   m.Dropped,
		})
	}
	pkglog.Infof("ReadAllMetrics: loaded %d metrics", len(out))
	return out, nil
}

// ---- Log reader ----

// readAllLogs reads all log parquet files from dir and returns LogData records.
func readAllLogs(dir string) ([]recorderdef.LogData, error) {
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

	var logs []recorderdef.LogData
	for _, f := range files {
		readLogParquetFile(f, &logs)
	}
	sort.Slice(logs, func(i, j int) bool { return logs[i].TimestampMs < logs[j].TimestampMs })
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
