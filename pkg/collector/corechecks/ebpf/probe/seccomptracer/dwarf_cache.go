// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package seccomptracer provides symbolication for stack traces
package seccomptracer

import (
	"debug/dwarf"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/utils/lru/simplelru"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/safeelf"
)

// binaryKey uniquely identifies a binary by its device and inode
type binaryKey struct {
	dev   uint64
	inode uint64
}

// binaryInfo contains parsed debug information for a binary
type binaryInfo struct {
	pathname   string
	dwarfData  *dwarf.Data      // Parsed DWARF (may be nil if stripped)
	elfFile    *safeelf.File    // ELF file handle
	symbols    []safeelf.Symbol // Sorted by address for fallback
	baseAddr   uint64           // Base virtual address for ET_EXEC files
	lastAccess time.Time        // For TTL expiration
}

// Close releases resources held by binaryInfo
func (bi *binaryInfo) Close() error {
	if bi.elfFile != nil {
		return bi.elfFile.Close()
	}
	return nil
}

// dwarfCache is a global cache of parsed binary debug info
type dwarfCache struct {
	mu         sync.RWMutex
	lru        *simplelru.LRU[binaryKey, *binaryInfo]
	pathToKey  map[string]binaryKey // Maps pathname to key for faster lookups
	ttl        time.Duration
	maxEntries int
}

var globalDwarfCache *dwarfCache

func init() {
	globalDwarfCache = newDwarfCache(100, 30*time.Second)
}

// newDwarfCache creates a new DWARF cache with the specified size and TTL
func newDwarfCache(maxEntries int, ttl time.Duration) *dwarfCache {
	lru, err := simplelru.NewLRU[binaryKey, *binaryInfo](
		maxEntries,
		func(key binaryKey, value *binaryInfo) {
			// Eviction callback - close resources
			if err := value.Close(); err != nil {
				log.Debugf("Failed to close binary info for key %+v: %v", key, err)
			}
		},
	)
	if err != nil {
		// This should never happen as we provide a positive size
		panic(fmt.Sprintf("Failed to create LRU cache: %v", err))
	}

	return &dwarfCache{
		lru:        lru,
		pathToKey:  make(map[string]binaryKey),
		ttl:        ttl,
		maxEntries: maxEntries,
	}
}

// get retrieves or loads binary information for the given key and pathname
func (dc *dwarfCache) get(key binaryKey, pathname string) (*binaryInfo, error) {
	// Fast path: check if we have a cached entry
	dc.mu.RLock()
	if info, ok := dc.lru.Get(key); ok {
		// Check if entry is still valid (TTL)
		if time.Since(info.lastAccess) < dc.ttl {
			info.lastAccess = time.Now()
			dc.mu.RUnlock()
			return info, nil
		}
		dc.mu.RUnlock()

		// Entry expired, remove it
		dc.mu.Lock()
		dc.lru.Remove(key)
		delete(dc.pathToKey, info.pathname)
		dc.mu.Unlock()
	} else {
		dc.mu.RUnlock()
	}

	// Slow path: load the binary
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have loaded it)
	if info, ok := dc.lru.Get(key); ok {
		if time.Since(info.lastAccess) < dc.ttl {
			info.lastAccess = time.Now()
			return info, nil
		}
		// Expired, remove it
		dc.lru.Remove(key)
		delete(dc.pathToKey, info.pathname)
	}

	// Load the binary
	info, err := dc.loadBinary(pathname)
	if err != nil {
		return nil, fmt.Errorf("failed to load binary %s: %w", pathname, err)
	}

	info.lastAccess = time.Now()

	// Add to cache
	dc.lru.Add(key, info)
	dc.pathToKey[pathname] = key

	return info, nil
}

// loadBinary loads debug information from a binary file
func (dc *dwarfCache) loadBinary(pathname string) (*binaryInfo, error) {
	// Open ELF file
	elfFile, err := safeelf.Open(pathname)
	if err != nil {
		return nil, fmt.Errorf("failed to open ELF file: %w", err)
	}

	info := &binaryInfo{
		pathname: pathname,
		elfFile:  elfFile,
	}

	// Compute base address for symbol lookup
	// For ET_EXEC files, symbols have virtual addresses, so we need to find the base
	// For ET_DYN files (PIE/shared libs), symbols are relative, so base is 0
	if elfFile.Type == safeelf.ET_EXEC {
		// Find the lowest virtual address from loadable segments
		info.baseAddr = ^uint64(0) // Max uint64
		for _, prog := range elfFile.Progs {
			if prog.Type == safeelf.PT_LOAD && prog.Vaddr < info.baseAddr {
				info.baseAddr = prog.Vaddr
			}
		}
		if info.baseAddr == ^uint64(0) {
			info.baseAddr = 0
		}
	}

	// Try to load DWARF data
	dwarfData, err := elfFile.DWARF()
	if err != nil {
		// DWARF not available (stripped binary), log once and continue
		log.Debugf("DWARF data not available for %s: %v", pathname, err)
	} else {
		info.dwarfData = dwarfData
	}

	// Load symbol table as fallback
	symbols, err := elfFile.Symbols()
	if err != nil {
		log.Debugf("Failed to load symbols for %s: %v", pathname, err)
		// Continue without symbols
	} else {
		// Filter and sort symbols by address
		var validSymbols []safeelf.Symbol
		for _, sym := range symbols {
			// Only keep function symbols with valid addresses
			if sym.Info&0xf == byte(safeelf.STT_FUNC) && sym.Value > 0 {
				validSymbols = append(validSymbols, sym)
			}
		}
		sort.Slice(validSymbols, func(i, j int) bool {
			return validSymbols[i].Value < validSymbols[j].Value
		})
		info.symbols = validSymbols
	}

	return info, nil
}

// Len returns the current number of entries in the cache
func (dc *dwarfCache) Len() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.lru.Len()
}

// Clear removes all entries from the cache
func (dc *dwarfCache) Clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.lru.Purge()
	dc.pathToKey = make(map[string]binaryKey)
}
