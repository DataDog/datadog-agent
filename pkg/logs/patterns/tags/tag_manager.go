// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags provides a thread-safe dictionary manager for encoding tag strings into dictionary indices
// for efficient storage and transmission in log pattern clustering.
package tags

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	dynamicStringDictionaryThreshold = 2
	maxPendingDynamicStrings         = 1_000_000
	minDynamicStringLength           = 2
	maxDynamicStringLength           = 128
)

// TagManager manages a dictionary of unique strings to dictionary IDs.
// It provides thread-safe operations for retrieving and creating IDs.
type TagManager struct {
	stringToEntry     map[string]*tagEntry
	idToEntry         map[uint64]*tagEntry
	pendingDynamic    map[string]uint16
	nextID            atomic.Uint64
	cachedMemoryBytes atomic.Int64
	mu                sync.RWMutex
}

// NewTagManager creates a new TagManager instance
func NewTagManager() *TagManager {
	tm := &TagManager{
		stringToEntry:  make(map[string]*tagEntry),
		idToEntry:      make(map[uint64]*tagEntry),
		pendingDynamic: make(map[string]uint16),
	}
	// Stateful Log reserves dict index 1 as the explicit-absence sentinel.
	tm.nextID.Store(1)
	return tm
}

// AddString adds a string to the dictionary and returns its dictionary ID
// If the string already exists, updates its access metadata and returns the existing ID
// Returns the dictionary ID and a boolean indicating if it was newly added
func (tm *TagManager) AddString(s string) (dictID uint64, isNew bool) {
	tm.mu.RLock()
	if entry, exists := tm.stringToEntry[s]; exists {
		tm.mu.RUnlock()
		// Update access metadata with write lock
		tm.mu.Lock()
		entry.usageCount++
		entry.lastAccessAt = time.Now()
		tm.mu.Unlock()
		return entry.id, false
	}
	tm.mu.RUnlock()

	tm.mu.Lock()
	defer tm.mu.Unlock()
	// Double-check locking in case another goroutine added it between locks
	if entry, exists := tm.stringToEntry[s]; exists {
		entry.usageCount++
		entry.lastAccessAt = time.Now()
		return entry.id, false
	}

	id := tm.nextID.Add(1)
	entry := &tagEntry{
		id:           id,
		str:          s,
		usageCount:   1,
		createdAt:    time.Now(),
		lastAccessAt: time.Now(),
	}
	tm.stringToEntry[s] = entry
	tm.idToEntry[id] = entry
	delete(tm.pendingDynamic, s)
	tm.cachedMemoryBytes.Add(entry.EstimatedBytes())
	return id, true
}

// ObserveDynamicString records a dynamic string value and returns a dictionary ID only
// after the string has repeated enough times to justify defining it. This lets dynamic
// values such as log levels reuse the dictionary while keeping one-off high-cardinality
// tokens inline.
func (tm *TagManager) ObserveDynamicString(s string) (dictID uint64, isNew bool, shouldEncode bool) {
	now := time.Now()

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if entry, exists := tm.stringToEntry[s]; exists {
		entry.usageCount++
		entry.lastAccessAt = now
		return entry.id, false, true
	}

	if !isDynamicStringDictionaryCandidate(s) {
		return 0, false, false
	}

	count, tracked := tm.pendingDynamic[s]
	if !tracked && len(tm.pendingDynamic) >= maxPendingDynamicStrings {
		return 0, false, false
	}
	if count < dynamicStringDictionaryThreshold {
		count++
	}
	if count < dynamicStringDictionaryThreshold {
		tm.pendingDynamic[s] = count
		return 0, false, false
	}

	id := tm.nextID.Add(1)
	entry := &tagEntry{
		id:           id,
		str:          s,
		usageCount:   int64(count),
		createdAt:    now,
		lastAccessAt: now,
	}
	tm.stringToEntry[s] = entry
	tm.idToEntry[id] = entry
	delete(tm.pendingDynamic, s)
	tm.cachedMemoryBytes.Add(entry.EstimatedBytes())
	return id, true, true
}

// GetStringID returns the dictionary ID for a string, if it exists
// Returns the ID and a boolean indicating if the string was found
func (tm *TagManager) GetStringID(s string) (uint64, bool) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	entry, exists := tm.stringToEntry[s]
	if !exists {
		return 0, false
	}
	return entry.id, true
}

// Get returns the dictionary ID for a tag string, if it exists
// Returns the ID and a boolean indicating if the tag was found
// Deprecated: Use GetStringID instead
func (tm *TagManager) Get(tag string) (uint64, bool) {
	return tm.GetStringID(tag)
}

// TouchDictID updates the access metadata for an existing dictionary ID.
// It returns false if the ID was already evicted.
func (tm *TagManager) TouchDictID(id uint64) bool {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	entry, exists := tm.idToEntry[id]
	if !exists {
		return false
	}
	entry.usageCount++
	entry.lastAccessAt = time.Now()
	return true
}

// EvictStaleEntries removes all entries that haven't been accessed within the given TTL.
// Returns the IDs of evicted entries so callers can send DictEntryDelete messages.
func (tm *TagManager) EvictStaleEntries(ttl time.Duration) []uint64 {
	cutoff := time.Now().Add(-ttl)

	tm.mu.Lock()
	defer tm.mu.Unlock()

	var evictedIDs []uint64
	for str, entry := range tm.stringToEntry {
		if entry.lastAccessAt.Before(cutoff) {
			evictedIDs = append(evictedIDs, entry.id)
			tm.cachedMemoryBytes.Add(-entry.EstimatedBytes())
			delete(tm.stringToEntry, str)
			delete(tm.idToEntry, entry.id)
		}
	}
	return evictedIDs
}

// Count returns the number of strings in the dictionary
func (tm *TagManager) Count() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.stringToEntry)
}

// HasDictID reports whether id is still present in the dictionary (not evicted).
func (tm *TagManager) HasDictID(id uint64) bool {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	_, ok := tm.idToEntry[id]
	return ok
}

func isDynamicStringDictionaryCandidate(s string) bool {
	if len(s) < minDynamicStringLength || len(s) > maxDynamicStringLength {
		return false
	}
	if looksLikeUUID(s) || looksLikeTimestamp(s) {
		return false
	}
	return true
}

func looksLikeUUID(s string) bool {
	if len(s) == 36 {
		for i := 0; i < len(s); i++ {
			switch i {
			case 8, 13, 18, 23:
				if s[i] != '-' {
					return false
				}
			default:
				if !isHex(s[i]) {
					return false
				}
			}
		}
		return true
	}
	if len(s) == 32 {
		for i := 0; i < len(s); i++ {
			if !isHex(s[i]) {
				return false
			}
		}
		return true
	}
	return false
}

func looksLikeTimestamp(s string) bool {
	if len(s) < len("2006-01-02") {
		return false
	}
	if !isDigit(s[0]) || !isDigit(s[1]) || !isDigit(s[2]) || !isDigit(s[3]) ||
		s[4] != '-' ||
		!isDigit(s[5]) || !isDigit(s[6]) ||
		s[7] != '-' ||
		!isDigit(s[8]) || !isDigit(s[9]) {
		return false
	}
	if len(s) == len("2006-01-02") {
		return true
	}
	switch s[10] {
	case 'T', ' ', '_':
		return true
	default:
		return false
	}
}

func isHex(c byte) bool {
	return isDigit(c) || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}
