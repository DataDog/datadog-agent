// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object

import "unsafe"

// NewDiskCacheInternal exports the newDiskCache function for testing.
func NewDiskCacheInternal(
	cfg DiskCacheConfig, diskUsageReader diskUsageReader,
) (*DiskCache, error) {
	return newDiskCache(cfg, diskUsageReader)
}

type DiskUsageReader = diskUsageReader

// EntryInfo contains detailed information about a cache entry.
type EntryInfo struct {
	RefCount  int
	DataStart uintptr // start address of the mapped data
	DataEnd   uintptr // end address of the mapped data
	DataSize  int     // size of the data in bytes
}

// EntryRefCounts returns a map of cache key strings to their current reference
// count.
func (d *DiskCache) EntryRefCounts() map[string]int {
	infos := d.EntryInfos()
	counts := make(map[string]int)
	for key, info := range infos {
		counts[key] = info.RefCount
	}
	return counts
}

// EntryInfos returns detailed information about all cache entries.
func (d *DiskCache) EntryInfos() map[string]EntryInfo {
	d.mu.Lock()
	defer d.mu.Unlock()
	infos := make(map[string]EntryInfo)
	for key, entry := range d.mu.entries {
		info := EntryInfo{
			RefCount: entry.cacheMu.refCount,
			DataSize: len(entry.decompress.data),
		}
		if len(entry.decompress.data) > 0 {
			info.DataStart = uintptr(unsafe.Pointer(&entry.decompress.data[0]))
			info.DataEnd = info.DataStart + uintptr(len(entry.decompress.data))
		}
		infos[key.String()] = info
	}
	return infos
}
