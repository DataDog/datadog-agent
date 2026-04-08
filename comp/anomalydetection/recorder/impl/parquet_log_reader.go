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

	for _, filePath := range r.files {
		r.readFile(filePath, &logs)
	}
	for _, filePath := range r.newFiles {
		r.readNewFormatLogFile(filePath, &logs)
	}

	// Sort by timestamp for consistent ordering
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].TimestampMs < logs[j].TimestampMs
	})

	return logs
}

func (r *LogParquetReader) readFile(filePath string, logs *[]recorderdef.LogData) {
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
		log := recorderdef.LogData{}

		if runIDCol != nil {
			log.Source = runIDCol.Value(i)
		}
		if timeCol != nil {
			log.TimestampMs = timeCol.Value(i)
		}
		if contentCol != nil && !contentCol.IsNull(i) {
			// Copy content data to avoid referencing memory that may be released
			data := contentCol.Value(i)
			log.Content = make([]byte, len(data))
			copy(log.Content, data)
		}
		if statusCol != nil {
			log.Status = statusCol.Value(i)
		}
		if hostnameCol != nil {
			log.Hostname = hostnameCol.Value(i)
		}
		if tagsCol != nil {
			log.Tags = readStringList(tagsCol, i)
		}

		*logs = append(*logs, log)
	}
}

// readNewFormatLogFile reads a logs-*.parquet file written by the new pipeline.
// Schema: hostname dict<string>, source dict<string>, status dict<string>,
//
//	tag_service dict<string>, tag_env dict<string>, tag_version dict<string>,
//	tag_team dict<string>, tags_overflow dict<string>, content binary, timestamp_ns int64.
//
// Uses file.ColumnChunkReader.ReadBatch directly (bypassing pqarrow) to avoid the
// arrow-go v18 double-configureDict bug that fires when reading dict-encoded columns
// across multiple row groups. Dictionary decoding is handled transparently by the
// file package's column chunk readers.
func (r *LogParquetReader) readNewFormatLogFile(filePath string, logs *[]recorderdef.LogData) {
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

	s := pf.MetaData().Schema
	hostnameIdx := findParquetColIdx(pf, "hostname")
	sourceIdx := findParquetColIdx(pf, "source")
	statusIdx := findParquetColIdx(pf, "status")
	tagServiceIdx := findParquetColIdx(pf, "tag_service")
	tagEnvIdx := findParquetColIdx(pf, "tag_env")
	tagVersionIdx := findParquetColIdx(pf, "tag_version")
	tagTeamIdx := findParquetColIdx(pf, "tag_team")
	tagsOverflowIdx := findParquetColIdx(pf, "tags_overflow")
	contentIdx := findParquetColIdx(pf, "content")
	tsIdx := findParquetColIdx(pf, "timestamp_ns")

	colMaxDef := func(idx int) int16 {
		if idx < 0 {
			return 0
		}
		return s.Column(idx).MaxDefinitionLevel()
	}

	hostnameMaxDef := colMaxDef(hostnameIdx)
	sourceMaxDef := colMaxDef(sourceIdx)
	statusMaxDef := colMaxDef(statusIdx)
	tagSvcMaxDef := colMaxDef(tagServiceIdx)
	tagEnvMaxDef := colMaxDef(tagEnvIdx)
	tagVerMaxDef := colMaxDef(tagVersionIdx)
	tagTeamMaxDef := colMaxDef(tagTeamIdx)
	tagsOvMaxDef := colMaxDef(tagsOverflowIdx)
	contentMaxDef := colMaxDef(contentIdx)
	tsMaxDef := colMaxDef(tsIdx)

	// readByteCol reads a ByteArray column (including dict-encoded strings) for a row group.
	readByteCol := func(rg *file.RowGroupReader, idx int, n int) ([]parquet.ByteArray, []int16) {
		if idx < 0 {
			return nil, nil
		}
		col, colErr := rg.Column(idx)
		if colErr != nil {
			return nil, nil
		}
		cr, ok := col.(*file.ByteArrayColumnChunkReader)
		if !ok {
			return nil, nil
		}
		buf := make([]parquet.ByteArray, n)
		def := make([]int16, n)
		if _, _, colErr = cr.ReadBatch(int64(n), buf, def, nil); colErr != nil {
			return nil, nil
		}
		return buf, def
	}

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		rgReader := pf.RowGroup(rg)
		numRows := int(rgReader.NumRows())
		if numRows == 0 {
			continue
		}

		hostBuf, hostDef := readByteCol(rgReader, hostnameIdx, numRows)
		srcBuf, srcDef := readByteCol(rgReader, sourceIdx, numRows)
		statusBuf, statusDef := readByteCol(rgReader, statusIdx, numRows)
		tagSvcBuf, tagSvcDef := readByteCol(rgReader, tagServiceIdx, numRows)
		tagEnvBuf, tagEnvDef := readByteCol(rgReader, tagEnvIdx, numRows)
		tagVerBuf, tagVerDef := readByteCol(rgReader, tagVersionIdx, numRows)
		tagTeamBuf, tagTeamDef := readByteCol(rgReader, tagTeamIdx, numRows)
		tagsOvBuf, tagsOvDef := readByteCol(rgReader, tagsOverflowIdx, numRows)
		contentBuf, contentDef := readByteCol(rgReader, contentIdx, numRows)

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

		// ReadBatch compacts non-null values; we track per-column value indices (vi)
		// to expand them back to row-aligned access using the definition levels.
		hostVI, srcVI, statusVI := 0, 0, 0
		tagSvcVI, tagEnvVI, tagVerVI, tagTeamVI, tagsOvVI := 0, 0, 0, 0, 0
		contentVI, tsVI := 0, 0

		// strAt reads the i-th row value from a compacted ByteArray buffer.
		strAt := func(buf []parquet.ByteArray, def []int16, maxD int16, vi *int, i int) string {
			if buf == nil || def[i] < maxD {
				return ""
			}
			v := string(buf[*vi])
			*vi++
			return v
		}

		for i := 0; i < numRows; i++ {
			hostname := strAt(hostBuf, hostDef, hostnameMaxDef, &hostVI, i)
			source := strAt(srcBuf, srcDef, sourceMaxDef, &srcVI, i)
			status := strAt(statusBuf, statusDef, statusMaxDef, &statusVI, i)

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
			addTag := func(buf []parquet.ByteArray, def []int16, maxD int16, vi *int, key string) {
				if buf == nil || def[i] < maxD {
					return
				}
				if v := string(buf[*vi]); v != "" {
					tags = append(tags, key+":"+v)
				}
				*vi++
			}
			addTag(tagSvcBuf, tagSvcDef, tagSvcMaxDef, &tagSvcVI, "service")
			addTag(tagEnvBuf, tagEnvDef, tagEnvMaxDef, &tagEnvVI, "env")
			addTag(tagVerBuf, tagVerDef, tagVerMaxDef, &tagVerVI, "version")
			addTag(tagTeamBuf, tagTeamDef, tagTeamMaxDef, &tagTeamVI, "team")

			if overflow := strAt(tagsOvBuf, tagsOvDef, tagsOvMaxDef, &tagsOvVI, i); overflow != "" {
				for _, t := range strings.Split(overflow, ",") {
					if t = strings.TrimSpace(t); t != "" {
						tags = append(tags, t)
					}
				}
			}

			logEntry := recorderdef.LogData{
				Source:      source,
				Status:      status,
				Hostname:    hostname,
				TimestampMs: tsMs,
				Content:     content,
			}
			if len(tags) > 0 {
				logEntry.Tags = tags
			}
			*logs = append(*logs, logEntry)
		}
	}
}
