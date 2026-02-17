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
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	recorderdef "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogParquetReader reads log data from parquet files.
type LogParquetReader struct {
	inputDir string
	files    []string
}

// NewLogParquetReader creates a reader for log parquet files.
func NewLogParquetReader(inputDir string) (*LogParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "observer-logs-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(inputDir, entry.Name()))
		}
	}

	// Sort files by name (which includes timestamp) for chronological order
	sort.Strings(files)

	return &LogParquetReader{
		inputDir: inputDir,
		files:    files,
	}, nil
}

// ReadAll reads all logs from all parquet files.
func (r *LogParquetReader) ReadAll() []recorderdef.LogData {
	var logs []recorderdef.LogData

	for _, filePath := range r.files {
		r.readFile(filePath, &logs)
	}

	// Sort by timestamp for consistent ordering
	sort.Slice(logs, func(i, j int) bool {
		return logs[i].Timestamp < logs[j].Timestamp
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

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, nil)
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
			log.Timestamp = timeCol.Value(i)
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
