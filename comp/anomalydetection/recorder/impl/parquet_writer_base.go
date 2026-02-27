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
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// batchBuilder builds an Arrow record from accumulated data.
// Returns nil if no data has been accumulated since the last build.
type batchBuilder interface {
	build() arrow.Record
}

// parquetWriterBase handles the common lifecycle for all parquet writers:
// flush loop, cleanup loop, atomic file creation per interval, and shutdown.
// Files are only created when there is data to write; empty files are never produced.
type parquetWriterBase struct {
	outputDir         string
	filePrefix        string // used for naming: <filePrefix>-<timestamp>Z.parquet
	schema            *arrow.Schema
	writerProps       *parquet.WriterProperties
	builder           batchBuilder
	flushInterval     time.Duration
	retentionDuration time.Duration // 0 means no cleanup
	stopCh            chan struct{}
	closed            bool
	mu                sync.Mutex
}

// start launches the background flush and cleanup goroutines.
func (b *parquetWriterBase) start() {
	go b.flushLoop()
	if b.retentionDuration > 0 {
		go b.cleanupLoop()
	}
}

// writeRecord creates a timestamped parquet file, writes the record, and closes it atomically.
// Only called when there is data; no file is created for empty batches.
// Must be called with b.mu held.
func (b *parquetWriterBase) writeRecord(record arrow.Record) error {
	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("%s-%sZ.parquet", b.filePrefix, timestamp)
	filePath := filepath.Join(b.outputDir, filename)

	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("creating parquet file %s: %w", filePath, err)
	}

	// WithStoreSchema embeds the Arrow schema into Parquet metadata,
	// enabling proper reconstruction of nested types like list<string>.
	arrowProps := pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema())

	writer, err := pqarrow.NewFileWriter(b.schema, file, b.writerProps, arrowProps)
	if err != nil {
		file.Close()
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	if err := writer.Write(record); err != nil {
		writer.Close()
		return fmt.Errorf("writing record to parquet: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}

	pkglog.Debugf("Wrote parquet file: %s (%d rows)", filePath, record.NumRows())
	return nil
}

// flush writes accumulated data to a new file if there is data to write.
// If no data has been collected since the last flush, no file is created.
func (b *parquetWriterBase) flush() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	record := b.builder.build()
	if record == nil {
		return
	}
	defer record.Release()

	if err := b.writeRecord(record); err != nil {
		pkglog.Errorf("Failed to flush %s to parquet: %v", b.filePrefix, err)
	}
}

func (b *parquetWriterBase) flushLoop() {
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			b.flush()
			return
		case <-ticker.C:
			b.flush()
		}
	}
}

func (b *parquetWriterBase) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.cleanup()
		}
	}
}

func (b *parquetWriterBase) cleanup() {
	entries, err := os.ReadDir(b.outputDir)
	if err != nil {
		pkglog.Warnf("Failed to read parquet output directory for cleanup: %v", err)
		return
	}

	cutoff := time.Now().Add(-b.retentionDuration)
	prefix := b.filePrefix + "-"
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), prefix) || !strings.HasSuffix(entry.Name(), ".parquet") {
			continue
		}

		filePath := filepath.Join(b.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			pkglog.Warnf("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				pkglog.Warnf("Failed to remove old parquet file %s: %v", filePath, err)
			} else {
				removed++
				pkglog.Debugf("Removed old parquet file: %s", filePath)
			}
		}
	}

	if removed > 0 {
		pkglog.Infof("Cleaned up %d old %s parquet file(s)", removed, b.filePrefix)
	}
}

// Close flushes remaining data and stops background goroutines.
func (b *parquetWriterBase) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}
	b.closed = true

	close(b.stopCh)

	record := b.builder.build()
	if record == nil {
		return nil
	}
	defer record.Release()

	if err := b.writeRecord(record); err != nil {
		return fmt.Errorf("final flush: %w", err)
	}
	return nil
}
