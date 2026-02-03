// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// ProfileParquetWriter writes observer profiles to rotating parquet files.
// Profile binary data is embedded directly in parquet using a LargeBinary column.
// This simplifies file management at the cost of slightly larger parquet files,
// which is acceptable for a recorder/replay use case.
type ProfileParquetWriter struct {
	outputDir         string
	currentFilePath   string
	writer            *pqarrow.FileWriter
	file              *os.File
	schema            *arrow.Schema
	builder           *profileBatchBuilder
	flushInterval     time.Duration
	retentionDuration time.Duration
	stopCh            chan struct{}
	closed            bool
	mu                sync.Mutex
}

// NewProfileParquetWriter creates a writer for profile data.
// Binary profile data is embedded directly in the parquet file using LargeBinary.
func NewProfileParquetWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*ProfileParquetWriter, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	// Schema for profiles with embedded binary data
	schema := arrow.NewSchema(
		[]arrow.Field{
			{Name: "RunID", Type: arrow.BinaryTypes.String},       // Source/namespace
			{Name: "Time", Type: arrow.PrimitiveTypes.Int64},      // Profile timestamp (ms since epoch)
			{Name: "ProfileID", Type: arrow.BinaryTypes.String},   // Unique profile identifier
			{Name: "ProfileType", Type: arrow.BinaryTypes.String}, // cpu, heap, mutex, etc.
			{Name: "Service", Type: arrow.BinaryTypes.String},
			{Name: "Env", Type: arrow.BinaryTypes.String},
			{Name: "Version", Type: arrow.BinaryTypes.String},
			{Name: "Hostname", Type: arrow.BinaryTypes.String},
			{Name: "ContainerID", Type: arrow.BinaryTypes.String},
			{Name: "DurationNs", Type: arrow.PrimitiveTypes.Int64},
			{Name: "ContentType", Type: arrow.BinaryTypes.String}, // Original Content-Type header
			{Name: "BinaryData", Type: arrow.BinaryTypes.Binary},  // Embedded profile binary (pprof, JFR, etc.)
			{Name: "Tags", Type: arrow.ListOf(arrow.BinaryTypes.String)},
		},
		nil,
	)

	pw := &ProfileParquetWriter{
		outputDir:         outputDir,
		schema:            schema,
		builder:           newProfileBatchBuilder(schema),
		flushInterval:     flushInterval,
		retentionDuration: retentionDuration,
		stopCh:            make(chan struct{}),
	}

	if err := pw.rotateFile(); err != nil {
		return nil, fmt.Errorf("creating initial parquet file: %w", err)
	}

	go pw.flushLoop()
	if retentionDuration > 0 {
		go pw.cleanupLoop()
	}

	pkglog.Infof("Profile parquet writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)

	return pw, nil
}

func (pw *ProfileParquetWriter) rotateFile() error {
	if pw.writer != nil {
		if err := pw.writer.Close(); err != nil {
			pkglog.Warnf("Error closing profile parquet writer during rotation: %v", err)
		}
		pw.writer = nil
	}
	pw.file = nil

	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("observer-profiles-%sZ.parquet", timestamp)
	pw.currentFilePath = filepath.Join(pw.outputDir, filename)

	file, err := os.Create(pw.currentFilePath)
	if err != nil {
		return fmt.Errorf("creating parquet file %s: %w", pw.currentFilePath, err)
	}

	props := parquet.NewWriterProperties(
		parquet.WithVersion(parquet.V2_LATEST),
		parquet.WithCompression(compress.Codecs.Zstd),
		parquet.WithBloomFilterEnabledFor("Service", true),
		parquet.WithBloomFilterFPPFor("Service", 0.01),
		parquet.WithBloomFilterEnabledFor("ProfileType", true),
		parquet.WithBloomFilterFPPFor("ProfileType", 0.01),
	)

	arrowProps := pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema())

	writer, err := pqarrow.NewFileWriter(pw.schema, file, props, arrowProps)
	if err != nil {
		file.Close()
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	pw.file = file
	pw.writer = writer

	pkglog.Debugf("Rotated to new profile parquet file: %s", pw.currentFilePath)

	return nil
}

// WriteProfile writes profile data (metadata + binary) to the parquet batch.
func (pw *ProfileParquetWriter) WriteProfile(
	source string,
	profileID, profileType string,
	service, env, version, hostname, containerID string,
	timestampNs, durationNs int64,
	contentType string,
	binaryData []byte,
	tags []string,
) {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	pw.builder.add(
		source,
		timestampNs/1000000, // Convert ns to ms
		profileID, profileType,
		service, env, version, hostname, containerID,
		durationNs,
		contentType,
		binaryData,
		tags,
	)
}

func (pw *ProfileParquetWriter) flushLoop() {
	ticker := time.NewTicker(pw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-pw.stopCh:
			pw.flushAndRotate()
			return
		case <-ticker.C:
			pw.flushAndRotate()
		}
	}
}

func (pw *ProfileParquetWriter) flushAndRotate() {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.closed {
		return
	}

	record := pw.builder.build()
	if record != nil {
		if err := pw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write profiles to parquet: %v", err)
		}
		record.Release()
	}

	if err := pw.rotateFile(); err != nil {
		pkglog.Errorf("Failed to rotate profile parquet file: %v", err)
	}
}

func (pw *ProfileParquetWriter) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-pw.stopCh:
			return
		case <-ticker.C:
			pw.cleanup()
		}
	}
}

func (pw *ProfileParquetWriter) cleanup() {
	entries, err := os.ReadDir(pw.outputDir)
	if err != nil {
		pkglog.Warnf("Failed to read profile parquet output directory for cleanup: %v", err)
		return
	}

	cutoff := time.Now().Add(-pw.retentionDuration)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "observer-profiles-") || !strings.HasSuffix(entry.Name(), ".parquet") {
			continue
		}

		filePath := filepath.Join(pw.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				pkglog.Warnf("Failed to remove old profile parquet file %s: %v", filePath, err)
			} else {
				removed++
			}
		}
	}

	if removed > 0 {
		pkglog.Infof("Cleaned up %d old profile parquet file(s)", removed)
	}
}

// Close flushes remaining data and closes the writer.
func (pw *ProfileParquetWriter) Close() error {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.closed {
		return nil
	}
	pw.closed = true

	close(pw.stopCh)

	record := pw.builder.build()
	if record != nil {
		if err := pw.writer.Write(record); err != nil {
			pkglog.Errorf("Failed to write final profiles to parquet: %v", err)
		}
		record.Release()
	}

	if pw.writer != nil {
		if err := pw.writer.Close(); err != nil {
			return fmt.Errorf("closing profile parquet writer: %w", err)
		}
		pw.writer = nil
	}
	pw.file = nil

	pkglog.Infof("Profile parquet writer closed: %s", pw.currentFilePath)
	return nil
}

// profileBatchBuilder accumulates profile data into Arrow record batches.
type profileBatchBuilder struct {
	schema *arrow.Schema

	runIDs       []string
	times        []int64
	profileIDs   []string
	profileTypes []string
	services     []string
	envs         []string
	versions     []string
	hostnames    []string
	containerIDs []string
	durations    []int64
	contentTypes []string
	binaryData   [][]byte // Embedded profile binary data
	tags         [][]string
}

func newProfileBatchBuilder(schema *arrow.Schema) *profileBatchBuilder {
	return &profileBatchBuilder{schema: schema}
}

func (b *profileBatchBuilder) add(
	source string,
	timeMs int64,
	profileID, profileType string,
	service, env, version, hostname, containerID string,
	durationNs int64,
	contentType string,
	binaryData []byte,
	tags []string,
) {
	b.runIDs = append(b.runIDs, source)
	b.times = append(b.times, timeMs)
	b.profileIDs = append(b.profileIDs, profileID)
	b.profileTypes = append(b.profileTypes, profileType)
	b.services = append(b.services, service)
	b.envs = append(b.envs, env)
	b.versions = append(b.versions, version)
	b.hostnames = append(b.hostnames, hostname)
	b.containerIDs = append(b.containerIDs, containerID)
	b.durations = append(b.durations, durationNs)
	b.contentTypes = append(b.contentTypes, contentType)

	// Copy binary data to avoid mutation
	dataCopy := make([]byte, len(binaryData))
	copy(dataCopy, binaryData)
	b.binaryData = append(b.binaryData, dataCopy)

	tagsCopy := make([]string, len(tags))
	copy(tagsCopy, tags)
	b.tags = append(b.tags, tagsCopy)
}

func (b *profileBatchBuilder) build() arrow.Record {
	if len(b.runIDs) == 0 {
		return nil
	}

	recordBuilder := array.NewRecordBuilder(memory.DefaultAllocator, b.schema)

	runIDBuilder := recordBuilder.Field(0).(*array.StringBuilder)
	timeBuilder := recordBuilder.Field(1).(*array.Int64Builder)
	profileIDBuilder := recordBuilder.Field(2).(*array.StringBuilder)
	profileTypeBuilder := recordBuilder.Field(3).(*array.StringBuilder)
	serviceBuilder := recordBuilder.Field(4).(*array.StringBuilder)
	envBuilder := recordBuilder.Field(5).(*array.StringBuilder)
	versionBuilder := recordBuilder.Field(6).(*array.StringBuilder)
	hostnameBuilder := recordBuilder.Field(7).(*array.StringBuilder)
	containerIDBuilder := recordBuilder.Field(8).(*array.StringBuilder)
	durationBuilder := recordBuilder.Field(9).(*array.Int64Builder)
	contentTypeBuilder := recordBuilder.Field(10).(*array.StringBuilder)
	binaryDataBuilder := recordBuilder.Field(11).(*array.BinaryBuilder)
	tagsBuilder := recordBuilder.Field(12).(*array.ListBuilder)
	tagsValueBuilder := tagsBuilder.ValueBuilder().(*array.StringBuilder)

	for _, v := range b.runIDs {
		runIDBuilder.Append(v)
	}
	timeBuilder.AppendValues(b.times, nil)
	for _, v := range b.profileIDs {
		profileIDBuilder.Append(v)
	}
	for _, v := range b.profileTypes {
		profileTypeBuilder.Append(v)
	}
	for _, v := range b.services {
		serviceBuilder.Append(v)
	}
	for _, v := range b.envs {
		envBuilder.Append(v)
	}
	for _, v := range b.versions {
		versionBuilder.Append(v)
	}
	for _, v := range b.hostnames {
		hostnameBuilder.Append(v)
	}
	for _, v := range b.containerIDs {
		containerIDBuilder.Append(v)
	}
	durationBuilder.AppendValues(b.durations, nil)
	for _, v := range b.contentTypes {
		contentTypeBuilder.Append(v)
	}
	for _, data := range b.binaryData {
		binaryDataBuilder.Append(data)
	}
	for _, tagList := range b.tags {
		tagsBuilder.Append(true)
		for _, tag := range tagList {
			tagsValueBuilder.Append(tag)
		}
	}

	record := recordBuilder.NewRecord()
	recordBuilder.Release()

	// Reset builder
	b.runIDs = nil
	b.times = nil
	b.profileIDs = nil
	b.profileTypes = nil
	b.services = nil
	b.envs = nil
	b.versions = nil
	b.hostnames = nil
	b.containerIDs = nil
	b.durations = nil
	b.contentTypes = nil
	b.binaryData = nil
	b.tags = nil

	return record
}
