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
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogParquetReader reads log data from parquet files.
type LogParquetReader struct {
	inputDir   string
	files      []string // legacy observer-logs-*.parquet files
	newFiles   []string // new-format logs-*.parquet files
}

// NewLogParquetReader creates a reader for log parquet files.
// It supports two file-name conventions:
//   - Legacy:  observer-logs-<timestamp>.parquet  (inline context columns)
//   - New:     logs-<timestamp>.parquet            (dictionary-encoded, timestamp_ns)
func NewLogParquetReader(inputDir string) (*LogParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	var files, newFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "observer-logs-") && strings.HasSuffix(name, ".parquet"):
			files = append(files, filepath.Join(inputDir, name))
		case strings.HasPrefix(name, "logs-") && strings.HasSuffix(name, ".parquet"):
			newFiles = append(newFiles, filepath.Join(inputDir, name))
		}
	}

	// Sort files by name (which includes timestamp) for chronological order
	sort.Strings(files)
	sort.Strings(newFiles)

	return &LogParquetReader{
		inputDir: inputDir,
		files:    files,
		newFiles: newFiles,
	}, nil
}

// ReadAll reads all logs from all parquet files.
func (r *LogParquetReader) ReadAll() []recorderdef.LogData {
	var logs []recorderdef.LogData
	_ = r.ForEachLog(func(l recorderdef.LogData) error {
		logs = append(logs, l)
		return nil
	})

	// Sort by timestamp for consistent ordering
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].TimestampMs < logs[j].TimestampMs
	})

	return logs
}

// ForEachLog streams all log entries without loading them all into memory.
// Entries are yielded in file order; files are processed in chronological
// order (sorted by filename). No cross-file sort is applied.
// Returning a non-nil error from fn stops iteration and propagates the error.
func (r *LogParquetReader) ForEachLog(fn func(recorderdef.LogData) error) error {
	for _, filePath := range r.files {
		if err := r.forEachLegacyLog(filePath, fn); err != nil {
			return err
		}
	}
	for _, filePath := range r.newFiles {
		if err := r.forEachNewFormatLog(filePath, fn); err != nil {
			return err
		}
	}
	return nil
}

func (r *LogParquetReader) forEachLegacyLog(filePath string, fn func(recorderdef.LogData) error) error {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open log parquet file %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		pkglog.Warnf("Failed to create parquet reader for %s: %v", filePath, err)
		return nil
	}
	defer pf.Close()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{BatchSize: 1024}, memory.DefaultAllocator)
	if err != nil {
		pkglog.Warnf("Failed to create arrow reader for %s: %v", filePath, err)
		return nil
	}

	table, err := reader.ReadTable(context.TODO())
	if err != nil {
		pkglog.Warnf("Failed to read table from %s: %v", filePath, err)
		return nil
	}
	defer table.Release()

	numRows := int(table.NumRows())
	if numRows == 0 {
		return nil
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
	getBinaryCol := func(name string) *array.Binary {
		if idx, ok := colIndices[name]; ok {
			chunks := table.Column(idx).Data().Chunks()
			if len(chunks) > 0 {
				return chunks[0].(*array.Binary)
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

	runIDCol := getStringCol("RunID")
	timeCol := getInt64Col("Time")
	contentCol := getBinaryCol("Content")
	statusCol := getStringCol("Status")
	hostnameCol := getStringCol("Hostname")
	tagsCol := getStringListCol("Tags")

	for i := 0; i < numRows; i++ {
		l := recorderdef.LogData{}

		if runIDCol != nil {
			l.Source = runIDCol.Value(i)
		}
		if timeCol != nil {
			l.TimestampMs = timeCol.Value(i)
		}
		if contentCol != nil && !contentCol.IsNull(i) {
			// Copy content data to avoid referencing memory that may be released
			data := contentCol.Value(i)
			l.Content = make([]byte, len(data))
			copy(l.Content, data)
		}
		if statusCol != nil {
			l.Status = statusCol.Value(i)
		}
		if hostnameCol != nil {
			l.Hostname = hostnameCol.Value(i)
		}
		if tagsCol != nil {
			l.Tags = readStringList(tagsCol, i)
		}

		if err := fn(l); err != nil {
			return err
		}
	}
	return nil
}

// forEachNewFormatLog reads a logs-*.parquet file written by the new pipeline,
// calling fn for each log entry.
//
// Only two columns have fixed semantics:
//   - content   (BYTE_ARRAY) → LogData.Content
//   - timestamp_ns (INT64)   → LogData.TimestampMs (divided by 1e6)
//
// Every other BYTE_ARRAY column becomes a tag in "column_name:value" form.
// This schema-agnostic approach means new columns added by the writer are
// automatically included as tags without any reader changes.
//
// Uses file.ColumnChunkReader.ReadBatch directly (bypassing pqarrow) to avoid the
// arrow-go v18 double-configureDict bug on multi-row-group dict-encoded columns.
func (r *LogParquetReader) forEachNewFormatLog(filePath string, fn func(recorderdef.LogData) error) error {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open log parquet file %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		pkglog.Warnf("Failed to create parquet reader for %s: %v", filePath, err)
		return nil
	}
	defer pf.Close()

	s := pf.MetaData().Schema
	numCols := s.NumColumns()

	contentIdx := findParquetColIdx(pf, "content")
	tsIdx := findParquetColIdx(pf, "timestamp_ns")

	// Classify every column as content, timestamp, or a tag column.
	type tagColMeta struct {
		idx    int
		name   string
		maxDef int16
	}
	var tagColsMeta []tagColMeta
	for i := 0; i < numCols; i++ {
		if i == contentIdx || i == tsIdx {
			continue
		}
		tagColsMeta = append(tagColsMeta, tagColMeta{
			idx:    i,
			name:   s.Column(i).Name(),
			maxDef: s.Column(i).MaxDefinitionLevel(),
		})
	}

	var contentMaxDef, tsMaxDef int16
	if contentIdx >= 0 {
		contentMaxDef = s.Column(contentIdx).MaxDefinitionLevel()
	}
	if tsIdx >= 0 {
		tsMaxDef = s.Column(tsIdx).MaxDefinitionLevel()
	}

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		rgReader := pf.RowGroup(rg)
		numRows := int(rgReader.NumRows())
		if numRows == 0 {
			continue
		}

		// Read timestamp_ns.
		var tsBuf []int64
		var tsDef []int16
		if tsIdx >= 0 {
			if col, colErr := rgReader.Column(tsIdx); colErr == nil {
				if cr, ok := col.(*file.Int64ColumnChunkReader); ok {
					tsBuf = make([]int64, numRows)
					tsDef = make([]int16, numRows)
					if _, _, colErr = cr.ReadBatch(int64(numRows), tsBuf, tsDef, nil); colErr != nil {
						tsBuf, tsDef = nil, nil
					}
				}
			}
		}

		// Read content.
		var contentBuf []parquet.ByteArray
		var contentDef []int16
		if contentIdx >= 0 {
			if col, colErr := rgReader.Column(contentIdx); colErr == nil {
				if cr, ok := col.(*file.ByteArrayColumnChunkReader); ok {
					contentBuf = make([]parquet.ByteArray, numRows)
					contentDef = make([]int16, numRows)
					if _, _, colErr = cr.ReadBatch(int64(numRows), contentBuf, contentDef, nil); colErr != nil {
						contentBuf, contentDef = nil, nil
					}
				}
			}
		}

		// Read all tag columns (any BYTE_ARRAY column that is not content/timestamp).
		type tagColData struct {
			tagColMeta
			buf []parquet.ByteArray
			def []int16
			vi  int
		}
		tagCols := make([]tagColData, 0, len(tagColsMeta))
		for _, meta := range tagColsMeta {
			col, colErr := rgReader.Column(meta.idx)
			if colErr != nil {
				continue
			}
			cr, ok := col.(*file.ByteArrayColumnChunkReader)
			if !ok {
				continue // skip non-BYTE_ARRAY columns (e.g. int, bool)
			}
			buf := make([]parquet.ByteArray, numRows)
			def := make([]int16, numRows)
			if _, _, colErr = cr.ReadBatch(int64(numRows), buf, def, nil); colErr != nil {
				continue
			}
			tagCols = append(tagCols, tagColData{tagColMeta: meta, buf: buf, def: def})
		}

		tsVI, contentVI := 0, 0

		for i := 0; i < numRows; i++ {
			var tsMs int64
			if tsBuf != nil && tsDef[i] >= tsMaxDef {
				tsMs = tsBuf[tsVI] / 1_000_000
				tsVI++
			}

			var content []byte
			if contentBuf != nil && contentDef[i] >= contentMaxDef {
				data := contentBuf[contentVI]
				content = make([]byte, len(data))
				copy(content, data)
				contentVI++
			}

			var tags []string
			for j := range tagCols {
				tc := &tagCols[j]
				if tc.def[i] >= tc.maxDef {
					if v := string(tc.buf[tc.vi]); v != "" {
						tags = append(tags, tc.name+":"+v)
					}
					tc.vi++
				}
			}

			if err := fn(recorderdef.LogData{
				TimestampMs: tsMs,
				Content:     content,
				Tags:        tags,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
