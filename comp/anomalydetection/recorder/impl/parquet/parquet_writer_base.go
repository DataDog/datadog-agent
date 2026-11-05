// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package parquet

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	"github.com/DataDog/datadog-agent/comp/anomalydetection/internal/logging"
)

// batchBuilder builds an Arrow record from accumulated data.
// Returns nil if no data has been accumulated since the last build.
type batchBuilder interface {
	build() arrow.RecordBatch
}

// parquetWriter shares flushing and retention across metric and log writers.
type parquetWriter struct {
	outputDir         string
	filePrefix        string // <filePrefix>-<timestamp>Z[_<sequence>].parquet
	schema            *arrow.Schema
	writerProps       *parquet.WriterProperties
	builder           batchBuilder
	flushInterval     time.Duration
	retentionDuration time.Duration // 0 means no cleanup
	stopCh            chan struct{}
	closed            bool
	closeErr          error
	mu                sync.Mutex
	workers           sync.WaitGroup
	now               func() time.Time
	openFile          func(string) (io.WriteCloser, error)
}

// start launches the background flush and cleanup goroutines.
func (b *parquetWriter) start() {
	b.workers.Add(1)
	go b.flushLoop()
	if b.retentionDuration > 0 {
		b.workers.Add(1)
		go b.cleanupLoop()
	}
}

// writeRecord writes a nonempty batch while b.mu is held.
func (b *parquetWriter) writeRecord(record arrow.RecordBatch) (err error) {
	baseName := fmt.Sprintf("%s-%sZ", b.filePrefix, b.now().UTC().Format("20060102-150405"))
	var (
		file      io.WriteCloser
		filePath  string
		tempPath  string
		published bool
	)
	for sequence := 0; ; sequence++ {
		filename := baseName + ".parquet"
		if sequence > 0 {
			filename = fmt.Sprintf("%s_%09d.parquet", baseName, sequence)
		}
		filePath = filepath.Join(b.outputDir, filename)
		tempPath = filePath + ".tmp"
		if _, statErr := os.Lstat(filePath); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return fmt.Errorf("checking parquet file %s: %w", filePath, statErr)
		}
		file, err = b.openTemporaryFile(tempPath)
		if errors.Is(err, os.ErrExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("creating parquet file %s: %w", filePath, err)
		}
		break
	}
	defer func() {
		if closeErr := file.Close(); closeErr != nil && !errors.Is(closeErr, os.ErrClosed) && err == nil {
			err = fmt.Errorf("closing parquet file: %w", closeErr)
		}
		if !published {
			_ = os.Remove(tempPath)
		}
	}()

	// WithStoreSchema embeds the Arrow schema into Parquet metadata,
	// enabling proper reconstruction of nested types like list<string>.
	arrowProps := pqarrow.NewArrowWriterProperties(pqarrow.WithStoreSchema())

	writer, err := pqarrow.NewFileWriter(b.schema, file, b.writerProps, arrowProps)
	if err != nil {
		return fmt.Errorf("creating parquet writer: %w", err)
	}

	if err := writer.Write(record); err != nil {
		writer.Close()
		return fmt.Errorf("writing record to parquet: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("closing parquet writer: %w", err)
	}
	if err := file.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
		return fmt.Errorf("closing parquet file: %w", err)
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		return fmt.Errorf("publishing parquet file %s: %w", filePath, err)
	}
	published = true

	logging.Debugf("Wrote parquet file: %s (%d rows)", filePath, record.NumRows())
	return nil
}

func (b *parquetWriter) openTemporaryFile(path string) (io.WriteCloser, error) {
	if b.openFile != nil {
		return b.openFile(path)
	}
	return os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o666)
}

// flush writes accumulated data to a new file if there is data to write.
// If no data has been collected since the last flush, no file is created.
func (b *parquetWriter) flush() {
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
		logging.Errorf("Failed to flush %s to parquet: %v", b.filePrefix, err)
	}
}

func (b *parquetWriter) flushLoop() {
	defer b.workers.Done()
	ticker := time.NewTicker(b.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-b.stopCh:
			return
		case <-ticker.C:
			b.flush()
		}
	}
}

func (b *parquetWriter) cleanupLoop() {
	defer b.workers.Done()
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

func (b *parquetWriter) cleanup() {
	if b.retentionDuration <= 0 {
		return
	}
	entries, err := os.ReadDir(b.outputDir)
	if err != nil {
		logging.Warnf("Failed to read parquet output directory for cleanup: %v", err)
		return
	}

	cutoff := b.now().Add(-b.retentionDuration)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !b.matchesFile(entry.Name()) {
			continue
		}

		filePath := filepath.Join(b.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			logging.Warnf("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				logging.Warnf("Failed to remove old parquet file %s: %v", filePath, err)
			} else {
				removed++
				logging.Debugf("Removed old parquet file: %s", filePath)
			}
		}
	}

	if removed > 0 {
		logging.Infof("Cleaned up %d old %s parquet file(s)", removed, b.filePrefix)
	}
}

func (b *parquetWriter) matchesFile(name string) bool {
	stem, ok := strings.CutPrefix(name, b.filePrefix+"-")
	if !ok {
		return false
	}
	stem, ok = strings.CutSuffix(stem, ".parquet")
	if !ok {
		return false
	}
	timestamp, sequence, hasSequence := strings.Cut(stem, "_")
	if _, err := time.Parse("20060102-150405Z", timestamp); err != nil {
		return false
	}
	if !hasSequence {
		return true
	}
	if len(sequence) != 9 {
		return false
	}
	for _, digit := range sequence {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

// Close flushes remaining data and joins background goroutines.
func (b *parquetWriter) Close() error {
	b.mu.Lock()
	if b.closed {
		err := b.closeErr
		b.mu.Unlock()
		b.workers.Wait()
		return err
	}
	b.closed = true
	close(b.stopCh)

	record := b.builder.build()
	if record != nil {
		b.closeErr = b.writeRecord(record)
		record.Release()
	}
	if b.closeErr != nil {
		b.closeErr = fmt.Errorf("final flush: %w", b.closeErr)
	}
	err := b.closeErr
	b.mu.Unlock()
	b.workers.Wait()
	return err
}
