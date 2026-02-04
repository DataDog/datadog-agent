// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
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

// ProfileParquetReader reads profile data from parquet files.
type ProfileParquetReader struct {
	inputDir string
	files    []string
}

// NewProfileParquetReader creates a reader for profile parquet files.
func NewProfileParquetReader(inputDir string) (*ProfileParquetReader, error) {
	entries, err := os.ReadDir(inputDir)
	if err != nil {
		return nil, err
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasPrefix(entry.Name(), "observer-profiles-") && strings.HasSuffix(entry.Name(), ".parquet") {
			files = append(files, filepath.Join(inputDir, entry.Name()))
		}
	}

	// Sort files by name (which includes timestamp) for chronological order
	sort.Strings(files)

	return &ProfileParquetReader{
		inputDir: inputDir,
		files:    files,
	}, nil
}

// ReadAll reads all profiles from all parquet files.
func (r *ProfileParquetReader) ReadAll() []recorderdef.ProfileData {
	var profiles []recorderdef.ProfileData

	for _, filePath := range r.files {
		profiles = append(profiles, r.readFile(filePath)...)
	}

	// Sort by timestamp for consistent ordering
	sort.Slice(profiles, func(i, j int) bool {
		return profiles[i].Timestamp < profiles[j].Timestamp
	})

	return profiles
}

func (r *ProfileParquetReader) readFile(filePath string) []recorderdef.ProfileData {
	f, err := os.Open(filePath)
	if err != nil {
		pkglog.Warnf("Failed to open profile parquet file %s: %v", filePath, err)
		return nil
	}
	defer f.Close()

	pf, err := file.NewParquetReader(f)
	if err != nil {
		pkglog.Warnf("Failed to create parquet reader for %s: %v", filePath, err)
		return nil
	}
	defer pf.Close()

	reader, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		pkglog.Warnf("Failed to create arrow reader for %s: %v", filePath, err)
		return nil
	}

	table, err := reader.ReadTable(nil)
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
	profileIDCol := getStringCol("ProfileID")
	profileTypeCol := getStringCol("ProfileType")
	serviceCol := getStringCol("Service")
	envCol := getStringCol("Env")
	versionCol := getStringCol("Version")
	hostnameCol := getStringCol("Hostname")
	containerIDCol := getStringCol("ContainerID")
	durationCol := getInt64Col("DurationNs")
	contentTypeCol := getStringCol("ContentType")
	binaryDataCol := getBinaryCol("BinaryData")
	tagsCol := getStringListCol("Tags")

	profiles := make([]recorderdef.ProfileData, 0, numRows)

	for i := 0; i < numRows; i++ {
		profile := recorderdef.ProfileData{}

		if runIDCol != nil {
			profile.Source = runIDCol.Value(i)
		}
		if timeCol != nil {
			profile.Timestamp = timeCol.Value(i) * 1000000 // Convert ms to ns
		}
		if profileIDCol != nil {
			profile.ProfileID = profileIDCol.Value(i)
		}
		if profileTypeCol != nil {
			profile.ProfileType = profileTypeCol.Value(i)
		}
		if serviceCol != nil {
			profile.Service = serviceCol.Value(i)
		}
		if envCol != nil {
			profile.Env = envCol.Value(i)
		}
		if versionCol != nil {
			profile.Version = versionCol.Value(i)
		}
		if hostnameCol != nil {
			profile.Hostname = hostnameCol.Value(i)
		}
		if containerIDCol != nil {
			profile.ContainerID = containerIDCol.Value(i)
		}
		if durationCol != nil {
			profile.Duration = durationCol.Value(i)
		}
		if contentTypeCol != nil {
			profile.ContentType = contentTypeCol.Value(i)
		}
		if binaryDataCol != nil && !binaryDataCol.IsNull(i) {
			// Copy binary data to avoid referencing memory that may be released
			data := binaryDataCol.Value(i)
			profile.BinaryData = make([]byte, len(data))
			copy(profile.BinaryData, data)
		}
		if tagsCol != nil {
			profile.Tags = readStringList(tagsCol, i)
		}

		profiles = append(profiles, profile)
	}

	return profiles
}
