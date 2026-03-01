// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package diskretry provides disk based log retry capabilities.
package diskretry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	tlmPayloadsWrittenToDisk = telemetry.NewCounterWithOpts("logs_sender", "disk_retry_payloads_written",
		[]string{}, "Payloads written to disk due to backpressure", telemetry.Options{DefaultMetric: true})
	tlmPayloadsReadFromDisk = telemetry.NewCounterWithOpts("logs_sender", "disk_retry_payloads_read",
		[]string{}, "Payloads read from disk for retry", telemetry.Options{DefaultMetric: true})
	tlmDiskWriteErrors = telemetry.NewCounterWithOpts("logs_sender", "disk_retry_write_errors",
		[]string{}, "Errors writing payloads to disk", telemetry.Options{DefaultMetric: true})
	tlmDiskReadErrors = telemetry.NewCounterWithOpts("logs_sender", "disk_retry_read_errors",
		[]string{}, "Errors reading payloads from disk", telemetry.Options{DefaultMetric: true})
	tlmDiskFull = telemetry.NewCounterWithOpts("logs_sender", "disk_retry_queue_full",
		[]string{}, "Payloads dropped due to disk queue being full", telemetry.Options{DefaultMetric: true})
	tlmDiskUsageBytes = telemetry.NewGaugeWithOpts("logs_sender", "disk_retry_usage_bytes",
		[]string{}, "Current disk usage in bytes for retry queue", telemetry.Options{DefaultMetric: true})
	tlmDiskFileCount = telemetry.NewGaugeWithOpts("logs_sender", "disk_retry_file_count",
		[]string{}, "Number of retry files on disk", telemetry.Options{DefaultMetric: true})
)

// Config represents the configuration options for the retrier
type Config struct {
	Enabled      bool   // Enable disk-based retry
	Path         string // Directory path for storing retry files
	MaxSizeBytes int64  // Maximum total size of all retry files
}

// Retrier takes takes payloads from the processor and writes them to disk
// whenever there is backpressure further in the logs pipeline. When the
// backpressure is relieved it sends them back into the pipeline.
type Retrier struct {
	config          Config
	mu              sync.Mutex
	done            chan struct{}
	currentDiskSize int64 // Track current disk usage
}

// PersistedPayload represents a payload that has been written to disk for retry
type PersistedPayload struct {
	MessageMetas  []*message.MessageMetadata `json:"message_metas"`
	Encoded       []byte                     `json:"encoded"`
	Encoding      string                     `json:"encoding"`
	UnencodedSize int                        `json:"unencoded_size"`

	CreatedAt int64  `json:"created_at"`
	WorkerID  string `json:"worker_id"`
}

// NewRetrier creates a new disk retrier
func NewRetrier(config Config) (*Retrier, error) {
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		return nil, err
	}

	return &Retrier{
		config: config,
	}, nil
}

// WritePayloadToDisk writes the payload to disk. The payload is structured JSON
// whose filename is a timestamp of when the file was created.
func (r *Retrier) WritePayloadToDisk(payload *message.Payload) (bool, error) {
	if r == nil || !r.config.Enabled {
		return false, nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Create persisted payload
	persisted := &PersistedPayload{
		MessageMetas:  payload.MessageMetas,
		Encoded:       payload.Encoded,
		Encoding:      payload.Encoding,
		UnencodedSize: payload.UnencodedSize,
		CreatedAt:     time.Now().Unix(),
		WorkerID:      "", // Can be set by caller if needed
	}

	// Marshal to JSON
	data, err := json.Marshal(persisted)
	if err != nil {
		return false, fmt.Errorf("failed to marshal payload: %w", err)
	}

	payloadSize := int64(len(data))

	// Check if we have capacity
	if r.config.MaxSizeBytes > 0 && r.currentDiskSize+payloadSize > r.config.MaxSizeBytes {
		log.Warnf("Disk retry storage is at max capacity (current: %d bytes, max: %d bytes), cannot write payload",
			r.currentDiskSize, r.config.MaxSizeBytes)
		tlmDiskFull.Inc()
		return false, fmt.Errorf("disk retry space full")
	}

	// Generate filename using forwarder convention: YYYY_MM_DD__HH_MM_SS_<random>.json
	filenamePrefix := time.Now().UTC().Format("2006_01_02__15_04_05_")
	tempFile, err := os.CreateTemp(r.config.Path, filenamePrefix+"*.json.tmp")
	if err != nil {
		tlmDiskWriteErrors.Inc()
		return false, fmt.Errorf("failed to create temp file: %w", err)
	}
	tempFilePath := tempFile.Name()

	// Write data to temp file
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		_ = os.Remove(tempFilePath)
		tlmDiskWriteErrors.Inc()
		return false, fmt.Errorf("failed to write to temp file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		_ = os.Remove(tempFilePath)
		tlmDiskWriteErrors.Inc()
		return false, fmt.Errorf("failed to close temp file: %w", err)
	}

	// Atomic rename: remove .tmp suffix
	finalPath := tempFilePath[:len(tempFilePath)-4] // Remove ".tmp"
	if err := os.Rename(tempFilePath, finalPath); err != nil {
		_ = os.Remove(tempFilePath)
		tlmDiskWriteErrors.Inc()
		return false, fmt.Errorf("failed to rename temp file: %w", err)
	}

	// Update disk usage tracking
	r.currentDiskSize += payloadSize
	tlmDiskUsageBytes.Set(float64(r.currentDiskSize))
	tlmPayloadsWrittenToDisk.Inc()

	log.Debugf("Successfully wrote payload to disk: %s (%d bytes)", filepath.Base(finalPath), payloadSize)
	return true, nil
}

// Stop stops the retrier and cleans up resources
func (r *Retrier) Stop() {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.done != nil {
		close(r.done)
	}

	log.Debug("Disk retrier stopped")
}

// readOldestPayload reads the oldest payload from disk, deletes the file, and
// returns the payload
func (r *Retrier) readOldestPayload() *message.Payload {
	if r == nil || !r.config.Enabled {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Read all files in the directory
	entries, err := os.ReadDir(r.config.Path)
	if err != nil {
		log.Warnf("Failed to read retry directory: %v", err)
		tlmDiskReadErrors.Inc()
		return nil
	}

	// Find oldest .json file by modification time
	var oldestFile string
	var oldestTime time.Time
	fileCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		fileCount++

		// Get file info for modification time
		info, err := entry.Info()
		if err != nil {
			continue
		}

		// First valid file or older than current oldest
		if oldestFile == "" || info.ModTime().Before(oldestTime) {
			oldestFile = name
			oldestTime = info.ModTime()
		}
	}

	// Update file count telemetry
	tlmDiskFileCount.Set(float64(fileCount))

	if oldestFile == "" {
		return nil // No files to replay
	}

	// Read the file
	filePath := filepath.Join(r.config.Path, oldestFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Warnf("Failed to read retry file %s: %v", oldestFile, err)
		tlmDiskReadErrors.Inc()
		return nil
	}

	// Unmarshal
	var persisted PersistedPayload
	if err := json.Unmarshal(data, &persisted); err != nil {
		log.Warnf("Failed to unmarshal retry file %s: %v", oldestFile, err)
		tlmDiskReadErrors.Inc()
		// Delete corrupted file
		_ = os.Remove(filePath)
		r.currentDiskSize -= int64(len(data))
		if r.currentDiskSize < 0 {
			r.currentDiskSize = 0
		}
		tlmDiskUsageBytes.Set(float64(r.currentDiskSize))
		return nil
	}

	// Delete the file since we've read it and will send it
	if err := os.Remove(filePath); err != nil {
		log.Warnf("Failed to delete retry file %s: %v", oldestFile, err)
		// Continue anyway - we'll try to send the payload
	}

	// Update disk size tracking
	r.currentDiskSize -= int64(len(data))
	if r.currentDiskSize < 0 {
		r.currentDiskSize = 0
	}
	tlmDiskUsageBytes.Set(float64(r.currentDiskSize))
	tlmPayloadsReadFromDisk.Inc()

	log.Debugf("Read payload from disk: %s (%d bytes)", oldestFile, len(data))

	// Convert to message.Payload
	return message.NewPayload(
		persisted.MessageMetas,
		persisted.Encoded,
		persisted.Encoding,
		persisted.UnencodedSize,
	)
}

// ReplayFromDisk tries to write take payloads on disk and sends them into the
// pipeline to be sent to the backend. It blocks if there is backpressure.
func (r *Retrier) ReplayFromDisk(payloadChan chan *message.Payload, done chan struct{}) {
	for {
		select {
		case <-done:
			return
		default:
			if payload := r.readOldestPayload(); payload != nil {
				// BLOCKING send - wait for space, never write back to disk
				payloadChan <- payload
			} else {
				time.Sleep(100 * time.Millisecond) // No files, wait
			}
		}
	}
}
