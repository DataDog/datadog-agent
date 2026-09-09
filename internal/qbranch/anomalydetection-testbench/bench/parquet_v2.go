// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

// V2 parquet reader for the deduplicated context layout.
//
// New format layout (as produced by the recorder on q-branch-observer):
//
//	contexts.parquet      — shared context table; loaded once per directory
//	logs-<ts>.parquet     — 3 cols: context_key (int64), content (bytes), timestamp_ns (int64)
//	metrics-<ts>.parquet  — 4 cols: context_key (int64), value (float64), timestamp_ns (int64), source (bytes)
//
// Uses file.ColumnChunkReader.ReadBatch (bypassing pqarrow) on log/metric files to
// avoid the arrow-go v18 double-configureDict bug on multi-row-group dict-encoded columns.
// contexts.parquet is read via pqarrow (single small file, no streaming needed).

import (
	"context"
	"fmt"
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

type contextEntryV2 struct {
	Name string
	Tags []string // pre-built "key:value" strings; callers must not mutate
}

// readContextsFileV2 loads contexts.parquet into a map keyed by context_key.
// Uses pqarrow (safe on this single small file — no multi-row-group dict issue).
func readContextsFileV2(filePath string) (map[uint64]contextEntryV2, error) {
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

	rr, err := arrowReader.GetRecordReader(context.Background(), nil, nil)
	if err != nil {
		return nil, fmt.Errorf("getting record reader: %w", err)
	}
	defer rr.Release()

	contexts := make(map[uint64]contextEntryV2)

	for rr.Next() {
		rec := rr.Record()
		schema := rec.Schema()

		ctxKeyIdx := findCol(schema, "context_key")
		nameIdx := findCol(schema, "name")
		tagHostIdx := findCol(schema, "tag_host")
		tagDeviceIdx := findCol(schema, "tag_device")
		tagSourceIdx := findCol(schema, "tag_source")
		tagServiceIdx := findCol(schema, "tag_service")
		tagEnvIdx := findCol(schema, "tag_env")
		tagVersionIdx := findCol(schema, "tag_version")
		tagTeamIdx := findCol(schema, "tag_team")
		tagsIdx := findCol(schema, "tags")

		if ctxKeyIdx < 0 || nameIdx < 0 {
			return nil, fmt.Errorf("contexts.parquet missing required columns context_key or name")
		}

		// context_key may be Uint64 (logical isSigned=false) or Int64 depending on
		// how the recorder wrote the file; normalise both to uint64.
		ctxKeyU64, _ := rec.Column(ctxKeyIdx).(*array.Uint64)
		ctxKeyI64, _ := rec.Column(ctxKeyIdx).(*array.Int64)
		nameCol, _ := rec.Column(nameIdx).(*array.String)

		getStr := func(idx int) *array.String {
			if idx < 0 {
				return nil
			}
			c, _ := rec.Column(idx).(*array.String)
			return c
		}
		tagHostCol := getStr(tagHostIdx)
		tagDeviceCol := getStr(tagDeviceIdx)
		tagSourceCol := getStr(tagSourceIdx)
		tagServiceCol := getStr(tagServiceIdx)
		tagEnvCol := getStr(tagEnvIdx)
		tagVersionCol := getStr(tagVersionIdx)
		tagTeamCol := getStr(tagTeamIdx)

		var tagsMapCol *array.Map
		if tagsIdx >= 0 {
			tagsMapCol, _ = rec.Column(tagsIdx).(*array.Map)
		}

		if ctxKeyU64 == nil && ctxKeyI64 == nil {
			return nil, fmt.Errorf("contexts.parquet: context_key column is neither uint64 nor int64")
		}

		isNull := func(i int) bool {
			if ctxKeyU64 != nil {
				return ctxKeyU64.IsNull(i)
			}
			return ctxKeyI64.IsNull(i)
		}
		keyAt := func(i int) uint64 {
			if ctxKeyU64 != nil {
				return ctxKeyU64.Value(i)
			}
			return uint64(ctxKeyI64.Value(i))
		}

		numRows := int(rec.NumRows())
		for i := 0; i < numRows; i++ {
			if isNull(i) {
				continue
			}
			key := keyAt(i)

			var name string
			if nameCol != nil && !nameCol.IsNull(i) {
				name = nameCol.Value(i)
			}

			tags := make([]string, 0, 8)
			addTag := func(col *array.String, tagKey string) {
				if col == nil || col.IsNull(i) {
					return
				}
				if v := col.Value(i); v != "" {
					tags = append(tags, tagKey+":"+v)
				}
			}
			addTag(tagHostCol, "host")
			addTag(tagDeviceCol, "device")
			addTag(tagSourceCol, "source")
			addTag(tagServiceCol, "service")
			addTag(tagEnvCol, "env")
			addTag(tagVersionCol, "version")
			addTag(tagTeamCol, "team")

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

			contexts[key] = contextEntryV2{Name: name, Tags: tags}
		}
	}

	if err := rr.Err(); err != nil && err.Error() != "EOF" {
		return nil, fmt.Errorf("reading contexts records: %w", err)
	}
	return contexts, nil
}

// findParquetColIdxV2 locates a column by normalized name in the parquet schema.
func findParquetColIdxV2(pf *file.Reader, name string) int {
	s := pf.MetaData().Schema
	n := normalizeCol(name)
	for i := 0; i < s.NumColumns(); i++ {
		if normalizeCol(s.Column(i).Name()) == n {
			return i
		}
	}
	return -1
}

// readAllMetricsV2 loads all metrics from a v2 parquet directory.
// contexts.parquet is loaded once; metric files are streamed row-group by row-group.
func readAllMetricsV2(dir string) ([]recorderdef.MetricData, error) {
	contexts, err := readContextsFileV2(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return nil, fmt.Errorf("reading contexts: %w", err)
	}

	files, err := findMetricParquetFilesV2(dir)
	if err != nil {
		return nil, err
	}

	var out []recorderdef.MetricData
	collect := func(m recorderdef.MetricData) error {
		out = append(out, m)
		return nil
	}
	for _, f := range files {
		if err := streamMetricFileV2(f, contexts, collect); err != nil {
			pkglog.Warnf("[parquet-v2] Skipping %s: %v", f, err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Timestamp < out[j].Timestamp })
	return out, nil
}

func findMetricParquetFilesV2(dir string) ([]string, error) {
	files, err := filepath.Glob(filepath.Join(dir, "metrics-*.parquet"))
	if err != nil {
		return nil, fmt.Errorf("listing metrics files: %w", err)
	}
	sort.Strings(files)
	return files, nil
}

func streamAllMetricsV2(dir string, fn func(string, recorderdef.MetricData) error) error {
	contexts, err := readContextsFileV2(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return fmt.Errorf("reading contexts: %w", err)
	}
	return streamAllMetricsV2WithContexts(dir, contexts, fn)
}

func streamAllMetricsV2WithContexts(dir string, contexts map[uint64]contextEntryV2, fn func(string, recorderdef.MetricData) error) error {
	files, err := findMetricParquetFilesV2(dir)
	if err != nil {
		return err
	}
	for _, filePath := range files {
		if err := streamMetricFileV2(filePath, contexts, func(metric recorderdef.MetricData) error {
			return fn(filePath, metric)
		}); err != nil {
			return fmt.Errorf("streaming %s: %w", filePath, err)
		}
	}
	return nil
}

func streamMetricFileV2(filePath string, contexts map[uint64]contextEntryV2, fn func(recorderdef.MetricData) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("statting %s: %w", filePath, err)
	}
	if info.Size() < minParquetFileSize {
		return fmt.Errorf("%s is too small to be parquet (%d bytes)", filePath, info.Size())
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
	ctxKeyIdx := findParquetColIdxV2(pf, "context_key")
	valueIdx := findParquetColIdxV2(pf, "value")
	tsIdx := findParquetColIdxV2(pf, "timestamp_ns")
	sourceIdx := findParquetColIdxV2(pf, "source")

	if ctxKeyIdx < 0 || valueIdx < 0 || tsIdx < 0 {
		return fmt.Errorf("missing one or more required columns: context_key, value, timestamp_ns")
	}

	ctxKeyMaxDef := s.Column(ctxKeyIdx).MaxDefinitionLevel()
	valueMaxDef := s.Column(valueIdx).MaxDefinitionLevel()
	tsMaxDef := s.Column(tsIdx).MaxDefinitionLevel()
	var srcMaxDef int16
	if sourceIdx >= 0 {
		srcMaxDef = s.Column(sourceIdx).MaxDefinitionLevel()
	}

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		rgReader := pf.RowGroup(rg)
		numRows := int(rgReader.NumRows())
		if numRows == 0 {
			continue
		}

		ctxKeyBuf := make([]int64, numRows)
		ctxKeyDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(ctxKeyIdx)
			if colErr != nil {
				continue
			}
			cr, ok := col.(*file.Int64ColumnChunkReader)
			if !ok {
				continue
			}
			if _, _, colErr = cr.ReadBatch(int64(numRows), ctxKeyBuf, ctxKeyDef, nil); colErr != nil {
				continue
			}
		}

		valueBuf := make([]float64, numRows)
		valueDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(valueIdx)
			if colErr != nil {
				continue
			}
			cr, ok := col.(*file.Float64ColumnChunkReader)
			if !ok {
				continue
			}
			if _, _, colErr = cr.ReadBatch(int64(numRows), valueBuf, valueDef, nil); colErr != nil {
				continue
			}
		}

		tsBuf := make([]int64, numRows)
		tsDef := make([]int16, numRows)
		{
			col, colErr := rgReader.Column(tsIdx)
			if colErr != nil {
				continue
			}
			cr, ok := col.(*file.Int64ColumnChunkReader)
			if !ok {
				continue
			}
			if _, _, colErr = cr.ReadBatch(int64(numRows), tsBuf, tsDef, nil); colErr != nil {
				continue
			}
		}

		var srcBuf []parquet.ByteArray
		var srcDef []int16
		if sourceIdx >= 0 {
			srcBuf = make([]parquet.ByteArray, numRows)
			srcDef = make([]int16, numRows)
			if col, colErr := rgReader.Column(sourceIdx); colErr == nil {
				if cr, ok := col.(*file.ByteArrayColumnChunkReader); ok {
					if _, _, colErr = cr.ReadBatch(int64(numRows), srcBuf, srcDef, nil); colErr != nil {
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
				Tags:      entry.Tags, // shared; do not mutate
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

// readAllLogsV2 loads all logs from a v2 parquet directory.
// contexts.parquet is loaded once; log files are streamed row-group by row-group.
func readAllLogsV2(dir string) ([]recorderdef.LogData, error) {
	contexts, err := readContextsFileV2(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return nil, fmt.Errorf("reading contexts: %w", err)
	}

	files, err := findLogParquetFilesV2(dir)
	if err != nil {
		return nil, err
	}

	var out []recorderdef.LogData
	collect := func(l recorderdef.LogData) error {
		out = append(out, l)
		return nil
	}
	for _, f := range files {
		if err := streamLogFileV2(f, contexts, collect); err != nil {
			pkglog.Warnf("[parquet-v2] Skipping %s: %v", f, err)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TimestampMs < out[j].TimestampMs })
	pkglog.Infof("[parquet-v2] Loaded %d logs from %s", len(out), dir)
	return out, nil
}

func findLogParquetFilesV2(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "logs-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(files)
	return files, nil
}

func streamAllLogsV2(dir string, fn func(string, recorderdef.LogData) error) error {
	contexts, err := readContextsFileV2(filepath.Join(dir, "contexts.parquet"))
	if err != nil {
		return fmt.Errorf("reading contexts: %w", err)
	}
	return streamAllLogsV2WithContexts(dir, contexts, fn)
}

func streamAllLogsV2WithContexts(dir string, contexts map[uint64]contextEntryV2, fn func(string, recorderdef.LogData) error) error {
	files, err := findLogParquetFilesV2(dir)
	if err != nil {
		return fmt.Errorf("listing v2 log parquet files: %w", err)
	}
	for _, filePath := range files {
		if err := streamLogFileV2(filePath, contexts, func(entry recorderdef.LogData) error {
			return fn(filePath, entry)
		}); err != nil {
			return fmt.Errorf("streaming %s: %w", filePath, err)
		}
	}
	return nil
}

func streamLogFileV2(filePath string, contexts map[uint64]contextEntryV2, fn func(recorderdef.LogData) error) error {
	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("statting %s: %w", filePath, err)
	}
	if info.Size() < minParquetFileSize {
		return fmt.Errorf("%s is too small to be parquet (%d bytes)", filePath, info.Size())
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
	ctxKeyIdx := findParquetColIdxV2(pf, "context_key")
	contentIdx := findParquetColIdxV2(pf, "content")
	tsIdx := findParquetColIdxV2(pf, "timestamp_ns")

	if ctxKeyIdx < 0 || contentIdx < 0 || tsIdx < 0 {
		return fmt.Errorf("missing one or more required columns: context_key, content, timestamp_ns")
	}

	var ctxKeyMaxDef, contentMaxDef, tsMaxDef int16
	ctxKeyMaxDef = s.Column(ctxKeyIdx).MaxDefinitionLevel()
	contentMaxDef = s.Column(contentIdx).MaxDefinitionLevel()
	tsMaxDef = s.Column(tsIdx).MaxDefinitionLevel()

	for rg := 0; rg < pf.NumRowGroups(); rg++ {
		rgReader := pf.RowGroup(rg)
		numRows := int(rgReader.NumRows())
		if numRows == 0 {
			continue
		}

		var ctxKeyBuf []int64
		var ctxKeyDef []int16
		if col, colErr := rgReader.Column(ctxKeyIdx); colErr == nil {
			if cr, ok := col.(*file.Int64ColumnChunkReader); ok {
				ctxKeyBuf = make([]int64, numRows)
				ctxKeyDef = make([]int16, numRows)
				if _, _, colErr = cr.ReadBatch(int64(numRows), ctxKeyBuf, ctxKeyDef, nil); colErr != nil {
					ctxKeyBuf, ctxKeyDef = nil, nil
				}
			}
		}

		var tsBuf []int64
		var tsDef []int16
		if col, colErr := rgReader.Column(tsIdx); colErr == nil {
			if cr, ok := col.(*file.Int64ColumnChunkReader); ok {
				tsBuf = make([]int64, numRows)
				tsDef = make([]int16, numRows)
				if _, _, colErr = cr.ReadBatch(int64(numRows), tsBuf, tsDef, nil); colErr != nil {
					tsBuf, tsDef = nil, nil
				}
			}
		}

		var contentBuf []parquet.ByteArray
		var contentDef []int16
		if col, colErr := rgReader.Column(contentIdx); colErr == nil {
			if cr, ok := col.(*file.ByteArrayColumnChunkReader); ok {
				contentBuf = make([]parquet.ByteArray, numRows)
				contentDef = make([]int16, numRows)
				if _, _, colErr = cr.ReadBatch(int64(numRows), contentBuf, contentDef, nil); colErr != nil {
					contentBuf, contentDef = nil, nil
				}
			}
		}

		ctxVI, tsVI, contentVI := 0, 0, 0
		for i := 0; i < numRows; i++ {
			ctxNull := ctxKeyBuf == nil || ctxKeyDef[i] < ctxKeyMaxDef
			var ctxKey uint64
			if !ctxNull {
				ctxKey = uint64(ctxKeyBuf[ctxVI])
				ctxVI++
			}

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

			if ctxNull {
				continue
			}
			entry, ok := contexts[ctxKey]
			if !ok {
				continue
			}

			// Extract hostname and source from the pre-built tag slice.
			var hostname, source string
			for _, tag := range entry.Tags {
				if strings.HasPrefix(tag, "host:") {
					hostname = tag[len("host:"):]
				} else if strings.HasPrefix(tag, "source:") {
					source = tag[len("source:"):]
				}
			}

			if err := fn(recorderdef.LogData{
				Source:      source,
				Hostname:    hostname,
				TimestampMs: tsMs,
				Content:     content,
				Tags:        entry.Tags, // shared; do not mutate
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
