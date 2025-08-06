// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package object_test

import (
	"iter"
	"os"
	"path"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/object"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/util/filesystem"
)

// TestDiskCacheHappyPath verifies that loading a binary through the disk cache
// returns identical DWARF debug sections compared to the in-memory loader and
// that the cache files are removed after the file is closed.
func TestDiskCacheHappyPath(t *testing.T) {
	// Obtain a test binary (simple) for the first common config available.
	cfgs := testprogs.MustGetCommonConfigs(t)
	require.NotEmpty(t, cfgs, "no testprogs configurations available")

	// Create a temporary directory to serve as the on-disk cache root.
	cacheDir := t.TempDir()

	// Instantiate the cache with generous limits so it should succeed.
	cache, err := object.NewDiskCache(object.DiskCacheConfig{
		DirPath:                  cacheDir,
		RequiredDiskSpaceBytes:   10 * 1024 * 1024,  // require 10 MiB free
		RequiredDiskSpacePercent: 1.0,               // 1% free space
		MaxTotalBytes:            512 * 1024 * 1024, // 512 MiB max cache size
	})
	require.NoError(t, err)
	programs := testprogs.MustGetPrograms(t)
	for _, program := range programs {
		t.Run(program, func(t *testing.T) {
			binaryPath := testprogs.MustGetBinary(t, program, cfgs[0])
			testDiskCache(t, cache, binaryPath, cacheDir)
		})
	}
}

func testDiskCache(
	t *testing.T,
	cache *object.DiskCache,
	binaryPath string,
	cacheDir string,
) {
	cachedObj, err := cache.Load(binaryPath)
	require.NoError(t, err)

	// Load the same binary with the standard in-memory loader.
	directObj, err := object.OpenElfFile(binaryPath)
	require.NoError(t, err)

	// Compare all debug section bytes.
	requireEqualDwarfSections(t, cachedObj, directObj)
	require.NoError(t, directObj.Close())

	// Ensure some files were created in cache directory.
	entries, err := os.ReadFile(path.Join("/", "proc", "self", "maps"))
	require.NoError(t, err)
	require.Greater(t, len(entries), 0, "expected cache files on disk")
	runtime.GC()
	procFS, err := procfs.Self()
	require.NoError(t, err)

	procMaps, err := procFS.ProcMaps()
	require.NoError(t, err)

	// Get detailed cache entry information
	entryInfos := cache.EntryInfos()
	require.NotEmpty(t, entryInfos, "expected cache entries")

	// Validate that cache entries have corresponding proc map entries
	validateCacheVsProcMaps(t, entryInfos, procMaps, cacheDir)

	// Ensure that the cache has the correct number of references to the cached object.
	for key, info := range entryInfos {
		require.Equal(t, 1, info.RefCount, "expected 1 reference count for %s", key)
	}

	reopenedObj, err := cache.Load(binaryPath)
	require.NoError(t, err)
	entryInfos = cache.EntryInfos()
	requireEqualDwarfSections(t, cachedObj, reopenedObj)
	for key, info := range entryInfos {
		require.Equal(t, 2, info.RefCount, "expected 2 reference count for %s", key)
	}
	runtime.KeepAlive(cachedObj) // after this line, cachedObj is gc-able
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		defer runtime.GC()
		entryInfos := cache.EntryInfos()
		for key, info := range entryInfos {
			require.Equal(c, 1, info.RefCount, "expected 1 reference count for %s", key)
		}
	}, 10*time.Second, time.Millisecond)
	runtime.KeepAlive(reopenedObj) // after this line, reopenedObj is gc-able
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		defer runtime.GC()
		entries, err := os.ReadDir(cacheDir)
		require.NoError(c, err)
		require.Len(c, entries, 0, "expected cache directory to be empty after close")
		require.Empty(c, cache.EntryInfos(), "expected no references to cached object")
		require.Zero(c, cache.SpaceInUse(), "expected no space in use")
	}, 10*time.Second, time.Millisecond)

	// Ensure that the maps are all gone too and that there are no files in the
	// cache directory
	procMaps, err = procFS.ProcMaps()
	require.NoError(t, err)
	validateCacheVsProcMaps(t, nil, procMaps, cacheDir)
}

func requireEqualDwarfSections(t *testing.T, a, b *object.ElfFile) {
	aSections, stopA := iter.Pull2(a.DwarfSections().Sections())
	defer stopA()
	bSections, stopB := iter.Pull2(b.DwarfSections().Sections())
	defer stopB()
	for {
		aName, aData, aOk := aSections()
		bName, bData, bOk := bSections()
		require.Equal(t, aOk, bOk, "section %s ok status differs", aName)
		if !aOk {
			break
		}
		require.Equal(t, aName, bName, "section names differ")
		require.Equal(t, aData, bData, "section %s differs", aName)
	}
}

// validateCacheVsProcMaps ensures that cache entries have corresponding proc
// map entries and that the address ranges align properly accounting for page
// boundaries.
func validateCacheVsProcMaps(t *testing.T, entryInfos map[string]object.EntryInfo, procMaps []*procfs.ProcMap, cacheDir string) {
	pageSize := uint64(os.Getpagesize())

	// Find proc maps that correspond to our cache files.
	var cacheProcMaps []*procfs.ProcMap
	for _, procMap := range procMaps {
		if strings.HasPrefix(procMap.Pathname, cacheDir) {
			cacheProcMaps = append(cacheProcMaps, procMap)
			t.Logf("cache proc map: 0x%x-0x%x %s", procMap.StartAddr, procMap.EndAddr, procMap.Pathname)
		}
	}

	// Validate each cache entry by finding the proc map that contains its address range.
	matchedProcMaps := make(map[*procfs.ProcMap]bool)
	for key, info := range entryInfos {
		t.Logf(
			"cache entry %s: data=0x%x-0x%x (%d bytes)",
			key, info.DataStart, info.DataEnd, info.DataSize,
		)

		// Find the proc map that contains this cache entry's data.
		idx := slices.IndexFunc(cacheProcMaps, func(procMap *procfs.ProcMap) bool {
			return info.DataStart >= uintptr(procMap.StartAddr) &&
				info.DataEnd <= uintptr(procMap.EndAddr)
		})
		require.GreaterOrEqual(t, idx, 0,
			"cache entry %s with data range 0x%x-0x%x not found in any proc map",
			key, info.DataStart, info.DataEnd,
		)
		matchingProcMap := cacheProcMaps[idx]
		matchedProcMaps[matchingProcMap] = true

		// Validate that the proc map boundaries are page-aligned as expected
		require.Zero(t, uint64(matchingProcMap.StartAddr)%pageSize,
			"proc map start 0x%x is not page-aligned (page size %d)", matchingProcMap.StartAddr, pageSize)
		require.Zero(t, uint64(matchingProcMap.EndAddr)%pageSize,
			"proc map end 0x%x is not page-aligned (page size %d)", matchingProcMap.EndAddr, pageSize)

		// Log the alignment details.
		dataOffset := info.DataStart - uintptr(matchingProcMap.StartAddr)
		t.Logf("cache entry %s: data offset within proc map: %d bytes (proc map: 0x%x-0x%x %s)",
			key, dataOffset, matchingProcMap.StartAddr, matchingProcMap.EndAddr, matchingProcMap.Pathname)

		// Ensure that the proc map path is deleted.
		require.True(t, strings.HasSuffix(matchingProcMap.Pathname, "(deleted)"),
			"proc map %s is not deleted", matchingProcMap.Pathname)
	}

	// Ensure all cache proc maps were matched to cache entries.
	require.Equal(t, len(cacheProcMaps), len(matchedProcMaps),
		"number of cache proc maps (%d) should match number of matched proc maps (%d)",
		len(cacheProcMaps), len(matchedProcMaps))
}

// stubDisk is a controllable implementation of the object.Disk interface for
// exercising disk-space limit logic.
type stubDisk struct {
	total     uint64
	available uint64
}

func (d stubDisk) ReadDiskUsage() (filesystem.DiskUsage, error) {
	return filesystem.DiskUsage{Total: d.total, Available: d.available}, nil
}

// TestDiskCacheLimits exercises the failure paths related to requiredDiskSpace,
// requiredDiskPercent, and maxTotalSize.
func TestDiskCacheLimits(t *testing.T) {
	cfgs := testprogs.MustGetCommonConfigs(t)
	require.NotEmpty(t, cfgs)

	binaryPath := testprogs.MustGetBinary(t, "sample", cfgs[0])

	// Common disk stats for most scenarios – 1 GiB total / available.
	const oneGiB = 1 << 30

	cases := []struct {
		name       string
		disk       stubDisk
		reqBytes   uint64
		reqPercent float64
		maxTotal   uint64
		expectErr  bool
		errorPat   string // regexp pattern that should match the error message
	}{
		{
			name:       "section_larger_than_available_space",
			disk:       stubDisk{total: oneGiB, available: 32 << 10}, // only 32 KiB available
			reqBytes:   1,                                            // 1 byte required after write
			reqPercent: 0.001,                                        // 0.001%
			maxTotal:   512 * 1024 * 1024,                            // 512 MiB
			expectErr:  true,
			errorPat:   "insufficient disk space: need",
		},
		{
			name:      "insufficient_free_bytes",
			disk:      stubDisk{total: oneGiB, available: oneGiB},
			reqBytes:  10 * oneGiB,       // unrealistic large requirement to guarantee failure
			maxTotal:  512 * 1024 * 1024, // 512 MiB
			expectErr: true,
			errorPat:  "disk space limits reached: need",
		},
		{
			name:       "insufficient_free_percent",
			disk:       stubDisk{total: oneGiB, available: oneGiB},
			reqBytes:   1024,              // 1 KiB
			reqPercent: 99.9,              // require 99.9% free after write -> impossible
			maxTotal:   512 * 1024 * 1024, // 512 MiB
			expectErr:  true,
			errorPat:   "disk space limits reached: need",
		},
		{
			name:      "exceeds_max_total_size",
			disk:      stubDisk{total: oneGiB, available: oneGiB},
			reqBytes:  1024, // 1 KiB
			maxTotal:  1,    // 1 byte max cache size, will be exceeded by first section
			expectErr: true,
			errorPat:  "would exceed cache size limit",
		},
		{
			name:       "within_limits",
			disk:       stubDisk{total: oneGiB, available: oneGiB},
			reqBytes:   1024,              // 1 KiB
			reqPercent: 0.1,               // 0.1%
			maxTotal:   512 * 1024 * 1024, // 512 MiB
			expectErr:  false,
		},
		{
			name:       "zero_limits",
			disk:       stubDisk{total: oneGiB, available: oneGiB},
			reqBytes:   0,
			reqPercent: 0,
			maxTotal:   512 * 1024 * 1024, // 512 MiB
			expectErr:  false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cacheDir := t.TempDir()
			cache, err := object.NewDiskCacheInternal(object.DiskCacheConfig{
				DirPath:                  cacheDir,
				RequiredDiskSpaceBytes:   tc.reqBytes,
				RequiredDiskSpacePercent: tc.reqPercent,
				MaxTotalBytes:            tc.maxTotal,
			}, tc.disk)
			require.NoError(t, err)

			obj, err := cache.Load(binaryPath)
			if tc.expectErr {
				require.Error(t, err, "expected error but got none")
				if tc.errorPat != "" {
					require.Regexp(t, tc.errorPat, err)
				}
				t.Logf("error: %v", err)
				return
			}
			require.NoError(t, err, "unexpected error")
			// Clean up to verify successful cases.
			require.NoError(t, obj.Close())
		})
	}
}
