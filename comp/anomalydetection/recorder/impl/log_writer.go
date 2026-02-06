// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package recorderimpl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// LogEntry represents a single log entry in JSON lines format.
type LogEntry struct {
	Timestamp int64    `json:"timestamp"`          // Unix timestamp in seconds
	Content   string   `json:"content"`            // Log message content
	Status    string   `json:"status,omitempty"`   // Log level (info, warn, error, etc.)
	Hostname  string   `json:"hostname,omitempty"` // Hostname where log originated
	Source    string   `json:"source,omitempty"`   // Source/namespace identifier
	Tags      []string `json:"tags,omitempty"`     // Tags in "key:value" format
}

// LogWriter writes observer logs to rotating JSON lines files.
// Files are rotated at the flush interval to ensure they remain valid and readable.
type LogWriter struct {
	outputDir         string
	currentFilePath   string
	file              *os.File
	encoder           *json.Encoder
	flushInterval     time.Duration
	retentionDuration time.Duration // 0 means no cleanup
	stopCh            chan struct{}
	closed            bool
	mu                sync.Mutex
}

// NewLogWriter creates a writer that rotates JSON lines files at the flush interval.
// outputDir: directory where log files will be written
// flushInterval: how often to rotate files (e.g., 60s creates a new file every minute)
// retentionDuration: how long to keep old files (0 = no cleanup)
func NewLogWriter(outputDir string, flushInterval, retentionDuration time.Duration) (*LogWriter, error) {
	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("creating output directory: %w", err)
	}

	lw := &LogWriter{
		outputDir:         outputDir,
		flushInterval:     flushInterval,
		retentionDuration: retentionDuration,
		stopCh:            make(chan struct{}),
	}

	// Create initial file
	if err := lw.rotateFile(); err != nil {
		return nil, fmt.Errorf("creating initial log file: %w", err)
	}

	// Start flush and cleanup loops
	go lw.flushLoop()
	if retentionDuration > 0 {
		go lw.cleanupLoop()
	}

	pkglog.Infof("Log writer initialized: dir=%s flush=%v retention=%v", outputDir, flushInterval, retentionDuration)

	return lw, nil
}

// rotateFile closes the current file and opens a new timestamped one
func (lw *LogWriter) rotateFile() error {
	// Close existing file
	if lw.file != nil {
		if err := lw.file.Sync(); err != nil {
			pkglog.Warnf("Error syncing log file during rotation: %v", err)
		}
		if err := lw.file.Close(); err != nil {
			pkglog.Warnf("Error closing log file during rotation: %v", err)
		}
		lw.file = nil
		lw.encoder = nil
	}

	// Generate timestamped filename with UTC timezone: observer-logs-20260129-133045Z.jsonl
	timestamp := time.Now().UTC().Format("20060102-150405")
	filename := fmt.Sprintf("observer-logs-%sZ.jsonl", timestamp)
	lw.currentFilePath = filepath.Join(lw.outputDir, filename)

	// Create new file
	file, err := os.Create(lw.currentFilePath)
	if err != nil {
		return fmt.Errorf("creating log file %s: %w", lw.currentFilePath, err)
	}

	lw.file = file
	lw.encoder = json.NewEncoder(file)

	pkglog.Debugf("Rotated to new log file: %s", lw.currentFilePath)

	return nil
}

// WriteLog adds a log entry to the current file
func (lw *LogWriter) WriteLog(timestamp int64, content string, tags []string, hostname, status, source string) {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	if lw.closed || lw.encoder == nil {
		return
	}

	entry := LogEntry{
		Timestamp: timestamp,
		Content:   content,
		Status:    status,
		Hostname:  hostname,
		Source:    source,
		Tags:      tags,
	}

	if err := lw.encoder.Encode(entry); err != nil {
		pkglog.Warnf("Failed to write log entry: %v", err)
	}
}

// flushLoop periodically flushes logs and rotates files
func (lw *LogWriter) flushLoop() {
	ticker := time.NewTicker(lw.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-lw.stopCh:
			lw.flushAndRotate()
			return
		case <-ticker.C:
			lw.flushAndRotate()
		}
	}
}

// flushAndRotate syncs current file and opens a new one
func (lw *LogWriter) flushAndRotate() {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Don't rotate if we're closing - Close() will handle final flush
	if lw.closed {
		return
	}

	// Sync current file before rotating
	if lw.file != nil {
		if err := lw.file.Sync(); err != nil {
			pkglog.Warnf("Failed to sync log file: %v", err)
		}
	}

	// Rotate to new file
	if err := lw.rotateFile(); err != nil {
		pkglog.Errorf("Failed to rotate log file: %v", err)
	}
}

// cleanupLoop periodically removes old log files beyond retention period
func (lw *LogWriter) cleanupLoop() {
	// Run cleanup every 5 minutes
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-lw.stopCh:
			return
		case <-ticker.C:
			lw.cleanup()
		}
	}
}

// cleanup removes log files older than retention duration
func (lw *LogWriter) cleanup() {
	entries, err := os.ReadDir(lw.outputDir)
	if err != nil {
		pkglog.Warnf("Failed to read log output directory for cleanup: %v", err)
		return
	}

	cutoff := time.Now().Add(-lw.retentionDuration)
	removed := 0

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		filePath := filepath.Join(lw.outputDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			pkglog.Warnf("Failed to get file info for %s: %v", filePath, err)
			continue
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filePath); err != nil {
				pkglog.Warnf("Failed to remove old log file %s: %v", filePath, err)
			} else {
				removed++
				pkglog.Debugf("Removed old log file: %s", filePath)
			}
		}
	}

	if removed > 0 {
		pkglog.Infof("Cleaned up %d old log file(s)", removed)
	}
}

// Close flushes remaining data and closes the writer
func (lw *LogWriter) Close() error {
	lw.mu.Lock()
	defer lw.mu.Unlock()

	// Check if already closed
	if lw.closed {
		return nil
	}
	lw.closed = true

	// Signal background goroutines to stop
	close(lw.stopCh)

	// Final flush and close
	if lw.file != nil {
		if err := lw.file.Sync(); err != nil {
			pkglog.Warnf("Failed to sync final log file: %v", err)
		}
		if err := lw.file.Close(); err != nil {
			return fmt.Errorf("closing log file: %w", err)
		}
		lw.file = nil
		lw.encoder = nil
	}

	pkglog.Infof("Log writer closed: %s", lw.currentFilePath)
	return nil
}
