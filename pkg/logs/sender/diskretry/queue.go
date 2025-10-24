// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package diskretry

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmDiskWrites       = telemetry.NewCounter("logs_diskretry", "writes", []string{}, "Payloads written to disk")
	tlmDiskWriteErrors  = telemetry.NewCounter("logs_diskretry", "write_errors", []string{}, "Errors writing to disk")
	tlmDiskReads        = telemetry.NewCounter("logs_diskretry", "reads", []string{}, "Payloads read from disk")
	tlmDiskReadErrors   = telemetry.NewCounter("logs_diskretry", "read_errors", []string{}, "Errors reading from disk")
	tlmDiskDeletes      = telemetry.NewCounter("logs_diskretry", "deletes", []string{}, "Payloads deleted from disk")
	tlmDiskBytesWritten = telemetry.NewCounter("logs_diskretry", "bytes_written", []string{}, "Bytes written to disk")
	tlmDiskBytesRead    = telemetry.NewCounter("logs_diskretry", "bytes_read", []string{}, "Bytes read from disk")
	tlmDiskFileCount    = telemetry.NewGauge("logs_diskretry", "file_count", []string{}, "Number of files on disk")
)

// Config holds configuration for the disk retry queue
type Config struct {
	Enabled         bool          // Enable disk-based retry
	Path            string        // Directory path for storing retry files
	MaxSizeBytes    int64         // Maximum total size of all retry files
	MaxAge          time.Duration // Maximum age of a retry file before it's dropped
	MaxRetries      int           // Maximum retry attempts per payload (0 = unlimited)
	CleanupInterval time.Duration // How often to scan for old files
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		Enabled:         false,
		Path:            "/opt/datadog-agent/run/logs-retry",
		MaxSizeBytes:    1024 * 1024 * 1024, // 1GB
		MaxAge:          24 * time.Hour,
		MaxRetries:      10,
		CleanupInterval: 5 * time.Minute,
	}
}

// Queue manages persistence of failed payloads to disk
type Queue struct {
	config Config
	mu     sync.Mutex
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewQueue creates a new disk retry queue
func NewQueue(config Config) (*Queue, error) {
	if !config.Enabled {
		return &Queue{config: config}, nil
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, fmt.Errorf("failed to create disk retry directory: %w", err)
	}

	q := &Queue{
		config: config,
		done:   make(chan struct{}),
	}

	// Start background cleanup goroutine
	q.wg.Add(1)
	go q.cleanupLoop()

	log.Infof("Disk retry queue enabled at %s (max size: %d bytes, max age: %s)",
		config.Path, config.MaxSizeBytes, config.MaxAge)

	return q, nil
}

// Add writes a payload to disk
func (q *Queue) Add(payload *message.Payload, workerID string) error {
	if !q.config.Enabled {
		return fmt.Errorf("disk retry queue is disabled")
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if we're over capacity
	currentSize, err := q.calculateDiskUsage()
	if err != nil {
		log.Warnf("Failed to calculate disk usage: %v", err)
	} else if currentSize >= q.config.MaxSizeBytes {
		tlmDiskWriteErrors.Inc()
		return fmt.Errorf("disk retry queue is full (size: %d bytes)", currentSize)
	}

	// Create persisted payload
	pp := FromPayload(payload, workerID)

	// Serialize to JSON
	data, err := pp.Marshal()
	if err != nil {
		tlmDiskWriteErrors.Inc()
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Generate filename: timestamp_workerID_count.json
	filename := fmt.Sprintf("%d_%s_%d.json",
		time.Now().UnixNano(),
		workerID,
		payload.Count())
	filepath := filepath.Join(q.config.Path, filename)

	// Write atomically (write to temp, then rename)
	tempPath := filepath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		tlmDiskWriteErrors.Inc()
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, filepath); err != nil {
		os.Remove(tempPath) // Clean up temp file
		tlmDiskWriteErrors.Inc()
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	tlmDiskWrites.Inc()
	tlmDiskBytesWritten.Add(float64(len(data)))
	log.Debugf("Wrote payload to disk: %s (%d bytes, %d messages)",
		filename, len(data), payload.Count())

	return nil
}

// List returns all persisted payloads on disk (oldest first)
func (q *Queue) List() ([]*PersistedPayload, error) {
	if !q.config.Enabled {
		return nil, nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	entries, err := os.ReadDir(q.config.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var payloads []*PersistedPayload
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		fullPath := filepath.Join(q.config.Path, entry.Name())
		data, err := os.ReadFile(fullPath)
		if err != nil {
			log.Warnf("Failed to read file %s: %v", entry.Name(), err)
			tlmDiskReadErrors.Inc()
			continue
		}

		pp, err := Unmarshal(data)
		if err != nil {
			log.Warnf("Failed to unmarshal file %s: %v", entry.Name(), err)
			tlmDiskReadErrors.Inc()
			continue
		}

		pp.FilePath = fullPath
		payloads = append(payloads, pp)
		tlmDiskReads.Inc()
		tlmDiskBytesRead.Add(float64(len(data)))
	}

	tlmDiskFileCount.Set(float64(len(payloads)))
	return payloads, nil
}

// Delete removes a persisted payload from disk
func (q *Queue) Delete(pp *PersistedPayload) error {
	if !q.config.Enabled {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	if err := os.Remove(pp.FilePath); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	tlmDiskDeletes.Inc()
	log.Debugf("Deleted payload from disk: %s", filepath.Base(pp.FilePath))
	return nil
}

// UpdateRetryMetadata updates the retry count and timestamp for a payload
func (q *Queue) UpdateRetryMetadata(pp *PersistedPayload) error {
	if !q.config.Enabled {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	pp.RetryCount++
	pp.LastRetryAt = time.Now().Unix()

	data, err := pp.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Atomic write
	tempPath := pp.FilePath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if err := os.Rename(tempPath, pp.FilePath); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// calculateDiskUsage returns total bytes used by retry files
func (q *Queue) calculateDiskUsage() (int64, error) {
	entries, err := os.ReadDir(q.config.Path)
	if err != nil {
		return 0, err
	}

	var total int64
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		total += info.Size()
	}

	return total, nil
}

// cleanupLoop periodically removes old/stale files
func (q *Queue) cleanupLoop() {
	defer q.wg.Done()

	ticker := time.NewTicker(q.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.cleanup()
		case <-q.done:
			return
		}
	}
}

// cleanup removes files that are too old or have too many retries
func (q *Queue) cleanup() {
	payloads, err := q.List()
	if err != nil {
		log.Warnf("Failed to list payloads during cleanup: %v", err)
		return
	}

	for _, pp := range payloads {
		if !pp.ShouldRetry(q.config.MaxAge, q.config.MaxRetries) {
			log.Infof("Removing stale payload: %s (age: %s, retries: %d)",
				filepath.Base(pp.FilePath), pp.Age(), pp.RetryCount)
			if err := q.Delete(pp); err != nil {
				log.Warnf("Failed to delete stale payload: %v", err)
			}
		}
	}
}

// GetConfig returns the queue's configuration
func (q *Queue) GetConfig() Config {
	return q.config
}

// Stop stops the cleanup loop
func (q *Queue) Stop() {
	if q.config.Enabled {
		close(q.done)
		q.wg.Wait()
		log.Info("Disk retry queue stopped")
	}
}
