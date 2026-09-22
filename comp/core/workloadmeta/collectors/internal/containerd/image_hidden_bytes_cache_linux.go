// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd && linux

package containerd

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/opencontainers/go-digest"
)

const (
	hiddenBytesAlgorithmVersion = 1
	hiddenBytesCacheEntries     = 128
	hiddenBytesCacheMaxBytes    = 2 << 20
	hiddenBytesCacheMaxLayers   = 4096
)

type hiddenBytesCacheEntry struct {
	ImageID string              `json:"image_id"`
	Layers  []hiddenLayerResult `json:"layers"`
}

type hiddenBytesCacheFile struct {
	Version int                     `json:"version"`
	Entries []hiddenBytesCacheEntry `json:"entries"`
}

// The cache stores results, never file names or contents. Entries are ordered
// least-recently-used first and are owned exclusively by the scan worker.
type hiddenBytesCache struct {
	path    string
	entries []hiddenBytesCacheEntry
}

func newHiddenBytesCache(path string) *hiddenBytesCache {
	return &hiddenBytesCache{path: path}
}

func (c *hiddenBytesCache) load() error {
	f, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, hiddenBytesCacheMaxBytes+1))
	if err != nil {
		return err
	}
	if len(data) > hiddenBytesCacheMaxBytes {
		return errors.New("hidden-byte cache exceeds size limit")
	}
	var stored hiddenBytesCacheFile
	if err := json.Unmarshal(data, &stored); err != nil {
		return err
	}
	if stored.Version != hiddenBytesAlgorithmVersion || len(stored.Entries) > hiddenBytesCacheEntries {
		return errors.New("unsupported hidden-byte cache version or entry count")
	}
	seen := make(map[string]bool, len(stored.Entries))
	for _, entry := range stored.Entries {
		if seen[entry.ImageID] || !validHiddenCacheEntry(entry) {
			return errors.New("invalid hidden-byte cache entry")
		}
		seen[entry.ImageID] = true
	}
	c.entries = stored.Entries
	return nil
}

func validHiddenCacheEntry(entry hiddenBytesCacheEntry) bool {
	if digest.Digest(entry.ImageID).Validate() != nil || len(entry.Layers) == 0 || len(entry.Layers) > hiddenBytesCacheMaxLayers {
		return false
	}
	for _, layer := range entry.Layers {
		if digest.Digest(layer.DiffID).Validate() != nil {
			return false
		}
	}
	return true
}

func (c *hiddenBytesCache) get(id string) ([]hiddenLayerResult, bool) {
	for i, entry := range c.entries {
		if entry.ImageID == id {
			c.entries = append(slices.Delete(c.entries, i, i+1), entry)
			return slices.Clone(entry.Layers), true
		}
	}
	return nil, false
}

func (c *hiddenBytesCache) put(id string, results []hiddenLayerResult) error {
	entry := hiddenBytesCacheEntry{ImageID: id, Layers: slices.Clone(results)}
	if !validHiddenCacheEntry(entry) {
		return errors.New("invalid hidden-byte cache result")
	}
	for i := range c.entries {
		if c.entries[i].ImageID == id {
			c.entries = slices.Delete(c.entries, i, i+1)
			break
		}
	}
	c.entries = append(c.entries, entry)
	if len(c.entries) > hiddenBytesCacheEntries {
		c.entries = slices.Delete(c.entries, 0, len(c.entries)-hiddenBytesCacheEntries)
	}
	var data []byte
	for {
		var err error
		data, err = json.Marshal(hiddenBytesCacheFile{Version: hiddenBytesAlgorithmVersion, Entries: c.entries})
		if err != nil {
			return err
		}
		if len(data) <= hiddenBytesCacheMaxBytes {
			break
		}
		c.entries = slices.Delete(c.entries, 0, 1)
	}
	if c.path == "" {
		return nil
	}
	dir := filepath.Dir(c.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".container-image-hidden-bytes-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), c.path)
}
