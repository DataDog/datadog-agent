// Package diskretry persists log payloads to disk while destinations are unavailable
// and replays them when connectivity recovers.
//
// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package diskretry

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	retryFileExtension = ".retry"
	// minSizeBytes is the minimum configurable disk budget. Prevents pathologically small limits.
	minSizeBytes = 2 * 1024 * 1024 // 2 MB
	// replayPollInterval is how often the replay loop checks for files when idle.
	replayPollInterval = 1 * time.Second
	// replayRetryInterval is how long the replay loop waits when it can't send (destination retrying).
	replayRetryInterval = 5 * time.Second
	// dirPermissions for the retry directory.
	dirPermissions = 0700
)

var (
	tlmPayloadsWritten = telemetry.NewCounter("disk_retry", "payloads_written", []string{}, "Payloads written to disk")
	tlmPayloadsRead    = telemetry.NewCounter("disk_retry", "payloads_read", []string{}, "Payloads read from disk for replay")
	tlmWriteErrors     = telemetry.NewCounter("disk_retry", "write_errors", []string{}, "Disk write failures")
	tlmReadErrors      = telemetry.NewCounter("disk_retry", "read_errors", []string{}, "Disk read/deserialization failures")
	tlmQueueFull       = telemetry.NewCounter("disk_retry", "queue_full", []string{}, "Payloads dropped because disk is at capacity")
	tlmUsageBytes      = telemetry.NewGauge("disk_retry", "usage_bytes", []string{}, "Current disk usage in bytes")
	tlmFileCount       = telemetry.NewGauge("disk_retry", "file_count", []string{}, "Number of retry files on disk")
)

// Retrier is the interface for the disk retry mechanism.
// Implementations: DiskRetryManager (enabled) and noopRetrier (disabled).
type Retrier interface {
	// Store writes a payload to disk. Returns nil on success.
	// Returns an error if the write fails (disk full, etc.) If so, the caller should drop the payload.
	Store(payload *message.Payload) error
	// StartReplayLoop starts a goroutine that replays retry files using the provided send function.
	StartReplayLoop(send SendFunc)
	// Stop signals the replay loop to stop and waits for it to exit.
	Stop()
}

// SendFunc is the function signature for attempting to send a payload to a destination.
// It mirrors DestinationSender.NonBlockingSend: returns true if the payload was accepted.
type SendFunc func(payload *message.Payload) bool

// DiskRetryManager writes payloads to disk when the destination is unreachable,
// and replays them when connectivity recovers.
type DiskRetryManager struct {
	storagePath  string
	maxSizeBytes int64
	maxDiskRatio float64
	fileTTLDays  int

	mu          sync.Mutex
	filenames   []string // FIFO ordered list of retry file paths
	currentSize int64    // total bytes of retry files on disk

	disk filesystem.Disk
	done chan struct{}
	wg   sync.WaitGroup
}

// NewDiskRetryManager creates a new DiskRetryManager. The storagePath directory
// is created if it doesn't exist. Existing retry files from previous runs are reloaded.
func NewDiskRetryManager(storagePath string, maxSizeBytes int64, maxDiskRatio float64, fileTTLDays int) (*DiskRetryManager, error) {
	if maxSizeBytes < minSizeBytes {
		maxSizeBytes = minSizeBytes
	}

	if err := os.MkdirAll(storagePath, dirPermissions); err != nil {
		return nil, fmt.Errorf("failed to create disk retry directory %s: %w", storagePath, err)
	}

	manager := &DiskRetryManager{
		storagePath:  storagePath,
		maxSizeBytes: maxSizeBytes,
		maxDiskRatio: maxDiskRatio,
		fileTTLDays:  fileTTLDays,
		disk:         filesystem.NewDisk(),
		done:         make(chan struct{}),
	}

	manager.reloadExistingFiles()
	return manager, nil
}

// Store serializes the payload and writes it atomically to disk.
func (manager *DiskRetryManager) Store(payload *message.Payload) error {
	data, err := SerializePayload(payload)
	if err != nil {
		tlmWriteErrors.Inc()
		return fmt.Errorf("serialization failed: %w", err)
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	// Expire old files before checking capacity
	manager.expireTTLLocked()

	// Check capacity
	if !manager.hasCapacityLocked(int64(len(data))) {
		tlmQueueFull.Inc()
		return fmt.Errorf("disk retry queue full (%d bytes used of %d max)", manager.currentSize, manager.maxSizeBytes)
	}

	// Atomic write: create temp file in same directory, write, close
	file, err := os.CreateTemp(manager.storagePath, fmt.Sprintf("%d_*%s", time.Now().UnixNano(), retryFileExtension))
	if err != nil {
		tlmWriteErrors.Inc()
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	if _, err := file.Write(data); err != nil {
		file.Close()
		os.Remove(file.Name())
		tlmWriteErrors.Inc()
		return fmt.Errorf("failed to write retry file: %w", err)
	}

	if err := file.Close(); err != nil {
		os.Remove(file.Name())
		tlmWriteErrors.Inc()
		return fmt.Errorf("failed to close retry file: %w", err)
	}

	manager.filenames = append(manager.filenames, file.Name())
	manager.currentSize += int64(len(data))
	manager.updateGauges()

	tlmPayloadsWritten.Inc()
	log.Debugf("Disk retry: wrote payload to %s (%d bytes, %d files on disk)", file.Name(), len(data), len(manager.filenames))
	return nil
}

// StartReplayLoop starts a goroutine that replays retry files using the provided send function.
// The send function should attempt a non-blocking send to the destination.
// Call Stop() to terminate the replay loop.
func (manager *DiskRetryManager) StartReplayLoop(send SendFunc) {
	manager.wg.Add(1)
	go manager.replayLoop(send)
}

// Stop signals the replay loop to exit and waits for it to finish.
func (manager *DiskRetryManager) Stop() {
	close(manager.done)
	manager.wg.Wait()
}

func (manager *DiskRetryManager) replayLoop(send SendFunc) {
	defer manager.wg.Done()

	for {
		select {
		case <-manager.done:
			return
		default:
		}

		payload, filePath := manager.readOldest()
		if payload == nil {
			// No files to replay; poll periodically
			select {
			case <-manager.done:
				return
			case <-time.After(replayPollInterval):
			}
			continue
		}

		// Try to send. If the destination is retrying (returns false),
		// back off and retry the same file.
		if send(payload) {
			// Success -> remove the file
			manager.mu.Lock()
			manager.removeFileLocked(filePath)
			manager.mu.Unlock()
			tlmPayloadsRead.Inc()
			log.Debugf("Disk retry: replayed and removed %s", filePath)
		} else {
			// Destination not ready -> put the file back and wait
			manager.mu.Lock()
			// Re-insert at the front (it was the oldest)
			manager.filenames = append([]string{filePath}, manager.filenames...)
			manager.mu.Unlock()

			select {
			case <-manager.done:
				return
			case <-time.After(replayRetryInterval):
			}
		}
	}
}

// readOldest reads and deserializes the oldest retry file.
// Returns (nil, "") if no files are available.
// The file is removed from the filenames list but NOT deleted from disk yet --
// the caller must call removeFileLocked on success or re-insert on failure.
func (manager *DiskRetryManager) readOldest() (*message.Payload, string) {
	manager.mu.Lock()
	if len(manager.filenames) == 0 {
		manager.mu.Unlock()
		return nil, ""
	}
	filePath := manager.filenames[0]
	manager.filenames = manager.filenames[1:]
	manager.mu.Unlock()

	data, err := os.ReadFile(filePath)
	if err != nil {
		log.Warnf("Disk retry: failed to read %s: %v", filePath, err)
		tlmReadErrors.Inc()
		// File is unreadable -> remove it from disk
		manager.mu.Lock()
		manager.removeFileLocked(filePath)
		manager.mu.Unlock()
		return nil, ""
	}

	payload, err := DeserializePayload(data)
	if err != nil {
		log.Warnf("Disk retry: failed to deserialize %s: %v", filePath, err)
		tlmReadErrors.Inc()
		// Corrupted file -> remove it
		manager.mu.Lock()
		manager.removeFileLocked(filePath)
		manager.mu.Unlock()
		return nil, ""
	}

	return payload, filePath
}

// removeFileLocked deletes a file from disk and updates the size counter.
// Must be called with manager.mu held.
func (manager *DiskRetryManager) removeFileLocked(filePath string) {
	size, err := filesystem.GetFileSize(filePath)
	if err == nil {
		manager.currentSize -= size
		if manager.currentSize < 0 {
			manager.currentSize = 0
		}
	}
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		log.Warnf("Disk retry: failed to remove %s: %v", filePath, err)
	}
	manager.updateGauges()
}

// hasCapacityLocked checks whether there is room for a file of the given size.
// Must be called with manager.mu held.
func (manager *DiskRetryManager) hasCapacityLocked(fileSize int64) bool {
	// Check configured limit
	if manager.currentSize+fileSize > manager.maxSizeBytes {
		return false
	}

	// Check filesystem capacity
	usage, err := manager.disk.GetUsage(manager.storagePath)
	if err != nil {
		log.Warnf("Disk retry: failed to check disk usage: %v", err)
		return false
	}
	diskReserved := float64(usage.Total) * (1.0 - manager.maxDiskRatio)
	available := int64(usage.Available) - int64(math.Ceil(diskReserved))
	return available >= fileSize
}

// expireTTLLocked removes files older than fileTTLDays. Must be called with manager.mu held.
func (manager *DiskRetryManager) expireTTLLocked() {
	if manager.fileTTLDays <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(manager.fileTTLDays) * 24 * time.Hour)
	var kept []string
	for _, f := range manager.filenames {
		modTime, err := filesystem.GetFileModTime(f)
		if err != nil {
			// Can't stat -> remove it
			manager.removeFileLocked(f)
			continue
		}
		if modTime.Before(cutoff) {
			log.Debugf("Disk retry: expiring old file %s (modified %s)", f, modTime)
			manager.removeFileLocked(f)
		} else {
			kept = append(kept, f)
		}
	}
	manager.filenames = kept
}

// reloadExistingFiles scans the storage directory for retry files from previous runs.
func (manager *DiskRetryManager) reloadExistingFiles() {
	entries, err := os.ReadDir(manager.storagePath)
	if err != nil {
		log.Warnf("Disk retry: failed to scan directory %s: %v", manager.storagePath, err)
		return
	}

	type fileInfo struct {
		path    string
		modTime time.Time
		size    int64
	}
	var files []fileInfo

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != retryFileExtension {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{
			path:    filepath.Join(manager.storagePath, entry.Name()),
			modTime: info.ModTime(),
			size:    info.Size(),
		})
	}

	// Sort by modification time (oldest first = FIFO)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.Before(files[j].modTime)
	})

	manager.mu.Lock()
	defer manager.mu.Unlock()
	for _, f := range files {
		manager.filenames = append(manager.filenames, f.path)
		manager.currentSize += f.size
	}
	manager.updateGauges()

	if len(files) > 0 {
		log.Infof("Disk retry: reloaded %d files (%d bytes) from %s", len(files), manager.currentSize, manager.storagePath)
	}
}

func (manager *DiskRetryManager) updateGauges() {
	tlmUsageBytes.Set(float64(manager.currentSize))
	tlmFileCount.Set(float64(len(manager.filenames)))
}

// noopRetrier is used when disk retry is disabled (max_size_bytes == 0).
type noopRetrier struct{}

// NewNoopRetrier returns a Retrier that always fails to store, causing the caller to drop the payload.
func NewNoopRetrier() Retrier {
	return &noopRetrier{}
}

func (n *noopRetrier) Store(_ *message.Payload) error {
	return errors.New("disk retry is disabled")
}

func (n *noopRetrier) StartReplayLoop(_ SendFunc) {}

func (n *noopRetrier) Stop() {}
