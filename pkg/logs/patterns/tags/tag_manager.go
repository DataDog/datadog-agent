// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package tags provides a thread-safe dictionary manager for encoding tag strings into dictionary indices
// for efficient storage and transmission in log pattern clustering.
package tags

import (
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/statefulpb"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TagManager manages a dictionary of unique tag strings (keys and values) to dictionary IDs.
// It provides thread-safe operations for retrieving/creating IDs and building Tag proto messages
// that reference those IDs.
type TagManager struct {
	stringToEntry    map[string]*tagEntry
	idToEntry        map[uint64]*tagEntry
	nextID           atomic.Uint64
	cachedMemoryBytes atomic.Int64
	mu               sync.RWMutex
}

// NewTagManager creates a new TagManager instance
func NewTagManager() *TagManager {
	tm := &TagManager{
		stringToEntry: make(map[string]*tagEntry),
		idToEntry:     make(map[uint64]*tagEntry),
	}
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
	tm.cachedMemoryBytes.Add(entry.EstimatedBytes())
	return id, true
}

// EncodeTagStrings converts a slice of "key:value" tag strings into Tag proto messages
// backed by dictionary indices. It returns the encoded tags plus the dictionary entries
// that must be flushed upstream (ID -> string) for any newly-seen key/value strings.
func (tm *TagManager) EncodeTagStrings(tagStrings []string) (tag []*statefulpb.Tag, dict map[uint64]string) {
	if len(tagStrings) == 0 {
		return []*statefulpb.Tag{}, map[uint64]string{}
	}

	encoded := make([]*statefulpb.Tag, 0, len(tagStrings))
	newEntries := map[uint64]string{}

	for _, tagStr := range tagStrings {
		if tagStr == "" {
			continue
		}

		key, value, hasDelimiter := strings.Cut(tagStr, ":")

		switch {
		case !hasDelimiter:
			// Treat bare value tags as key-only tags.
			keyID, keyNew := tm.AddString(tagStr)
			if keyNew {
				newEntries[keyID] = tagStr
			}
			encoded = append(encoded, &statefulpb.Tag{
				Key: dictIndexValue(keyID),
			})
		case key == "":
			// Assume that user mistype the tag string, skip it.
			log.Warnf("Invalid tag string: %s", tagStr)
			continue
		default:
			keyID, keyNew := tm.AddString(key)
			if keyNew {
				newEntries[keyID] = key
			}

			tag := &statefulpb.Tag{
				Key: dictIndexValue(keyID),
			}

			// Only add value to dictionary if it's not empty
			if value != "" {
				valueID, valueNew := tm.AddString(value)
				if valueNew {
					newEntries[valueID] = value
				}
				tag.Value = dictIndexValue(valueID)
			}

			encoded = append(encoded, tag)
		}
	}

	return encoded, newEntries
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

// Count returns the number of strings in the dictionary
func (tm *TagManager) Count() int {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.stringToEntry)
}

// dictIndexValue converts a dictionary ID to a DynamicValue proto message
func dictIndexValue(id uint64) *statefulpb.DynamicValue {
	return &statefulpb.DynamicValue{
		Value: &statefulpb.DynamicValue_DictIndex{
			DictIndex: id,
		},
	}
}
