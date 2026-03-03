// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// entryType represents the type of upload entry.
type entryType int

const (
	// entryTypeAttempt represents an upload attempt entry. An attempt is
	// inserted in the cache before an upload is started; the entry is then
	// converted to a completed one if the upload is successful.
	entryTypeUnknown entryType = iota
	entryTypeAttempt
	// entryTypeCompleted represents a completed upload entry.
	entryTypeCompleted
)

// uploadEntry represents an entry in the upload cache.
type uploadEntry struct {
	Type           entryType `json:"type"`
	PID            int32     `json:"pid"`
	ServiceName    string    `json:"servicename"`
	ServiceVersion string    `json:"serviceversion"`
	// AgentVersion is the version of the agent that wrote this entry.
	AgentVersion string    `json:"agentversion"`
	Timestamp    time.Time `json:"timestamp"`
	// ErrorNumber counts how many times that upload failed.
	ErrorNumber int    `json:"errornumber,omitempty"` // Only used for entryTypeAttempt
	Error       string `json:"error,omitempty"`       // Optional error message for failed attempts
}

// persistentUploadCache stores information about attempted and completed debug
// info processing and uploads to SymDB for different processes. The info is
// stored on disk so it persists across agent restarts. Information about past
// upload attempts can be used after a restart to decide if uploads should be
// attempted again.
type persistentUploadCache struct {
	// The directory in which the cache entries are stored (each file in this
	// dir represents one entry).
	dir string
}

// AddAttempt adds a new upload attempt entry to the cache. If the entry for the
// pid already exists, it is updated.
func (c *persistentUploadCache) AddAttempt(
	pid int32, serviceName, serviceVersion string, errorNumber int, errMsg string,
) error {
	entry := uploadEntry{
		Type:           entryTypeAttempt,
		PID:            pid,
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		AgentVersion:   version.AgentVersion,
		Timestamp:      time.Now(),
		ErrorNumber:    errorNumber,
	}
	if errMsg != "" {
		entry.Error = errMsg
	}
	return c.saveEntry(pid, entry)
}

// AddCompleted adds a new completed upload entry to the cache.
func (c *persistentUploadCache) AddCompleted(pid int32, serviceName, serviceVersion string) error {
	entry := uploadEntry{
		Type:           entryTypeCompleted,
		PID:            pid,
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		AgentVersion:   version.AgentVersion,
		Timestamp:      time.Now(),
	}
	return c.saveEntry(pid, entry)
}

// GetEntry retrieves an upload entry for the given PID.
func (c *persistentUploadCache) GetEntry(pid int32) (*uploadEntry, error) {
	path := c.entryPath(pid)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read entry file: %w", err)
	}

	var entry uploadEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	return &entry, nil
}

// RemoveEntry removes the entry for the given PID.
func (c *persistentUploadCache) RemoveEntry(pid int32) error {
	path := c.entryPath(pid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove entry: %w", err)
	}
	return nil
}

type cacheTestingKnobs struct {
	processExists func(pid int) bool
}

type cacheOption func(*cacheTestingKnobs)

func withProcessExistsCheck(fn func(pid int) bool) cacheOption {
	return func(c *cacheTestingKnobs) {
		c.processExists = fn
	}
}

// NewPersistentUploadCache initializes a cache in the given directory. If the
// directory already exists, its state is update to match the current state of
// the processes on the box: files corresponding to processes that no longer
// exist are deleted and files corresponding to pending uploads for processes
// that do exist are updated with an error to mark the fact that the agent
// restarted while that update was in progress.
func newPersistentUploadCache(dir string, opts ...cacheOption) (*persistentUploadCache, error) {
	// Ensure the cache directory exists or can be created.
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	cfg := cacheTestingKnobs{}
	for _, opt := range opts {
		opt(&cfg)
	}

	cache := &persistentUploadCache{
		dir: dir,
	}

	entries, err := os.ReadDir(cache.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return cache, nil // Directory doesn't exist yet, no entries to clean.
		}
		return nil, fmt.Errorf("failed to read cache directory: %w", err)
	}

	removedVersionMismatch := 0
	for _, entry := range entries {
		filePath := filepath.Join(cache.dir, entry.Name())
		name := entry.Name()

		if entry.IsDir() {
			// Delete unexpected directories.
			if err := os.RemoveAll(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove directory %s: %w", name, err)
			}
			continue
		}

		// Check if the filename has the expected format.
		if !strings.HasSuffix(name, ".json") {
			// Delete files with invalid extensions.
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove invalid file %s: %w", name, err)
			}
			continue
		}

		// Parse PID from filename (e.g., "1234.json" -> 1234).
		pidStr := strings.TrimSuffix(name, ".json")
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			// Delete files with invalid names.
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove file with invalid name %s: %w", name, err)
			}
			continue
		}

		// Try to read and parse the file contents.
		data, err := os.ReadFile(filePath)
		if err != nil {
			// Delete files that can't be read.
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove unreadable file %s: %w", name, err)
			}
			continue
		}

		var uploadEntry uploadEntry
		if err := json.Unmarshal(data, &uploadEntry); err != nil {
			// Delete files with invalid JSON contents.
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove file with invalid JSON %s: %w", name, err)
			}
			continue
		}

		// Validate that the entry type is a known enum value.
		if uploadEntry.Type != entryTypeAttempt && uploadEntry.Type != entryTypeCompleted {
			// Delete files with invalid entry type.
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove file with invalid entry type %s: %w", name, err)
			}
			continue
		}

		// Delete entries from different agent versions; we want to upload again
		// when the agent version changes.
		if uploadEntry.AgentVersion != version.AgentVersion {
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove entry with old version for PID %d: %w", pid, err)
			}
			removedVersionMismatch++
			continue
		}

		// Check if the process still exists.
		var processExists bool
		if cfg.processExists != nil {
			processExists = cfg.processExists(pid)
		} else {
			_, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid)))
			processExists = !os.IsNotExist(err)
		}

		if !processExists {
			log.Debugf("Removing stale cache entry for PID %d (process no longer exists)", pid)
			if err := os.Remove(filePath); err != nil {
				return nil, fmt.Errorf("failed to remove stale entry for PID %d: %w", pid, err)
			}
			continue
		}

		// For processes that still exist, mark pending uploads as failed due to restart.
		if uploadEntry.Type == entryTypeAttempt {
			uploadEntry.Error = "agent restarted during upload"
			if err := cache.saveEntry(int32(pid), uploadEntry); err != nil {
				return nil, fmt.Errorf("failed to update entry for PID %d: %w", pid, err)
			}
		}
	}

	if removedVersionMismatch > 0 {
		log.Infof("Removed %d cache entries created by a different agent version (current version: %s)",
			removedVersionMismatch, version.AgentVersion)
	}

	return cache, nil
}

func (c *persistentUploadCache) saveEntry(pid int32, entry uploadEntry) error {
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}
	entryPath := c.entryPath(pid)
	if err := os.WriteFile(entryPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache entry file: %w", err)
	}
	return nil
}

func (c *persistentUploadCache) entryPath(pid int32) string {
	return filepath.Join(c.dir, fmt.Sprintf("%d.json", pid))
}
