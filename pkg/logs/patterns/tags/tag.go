// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"time"
)

// tagEntry represents a dictionary entry with usage tracking metadata for eviction.
type tagEntry struct {
	id           uint64
	str          string
	usageCount   int64
	createdAt    time.Time
	lastAccessAt time.Time
}

// EstimatedBytes returns the estimated memory usage of a tag entry
func (e *tagEntry) EstimatedBytes() int64 {
	// id uint64 (8) + string (16 bytes header + string length) + usage count int64 (8) + 2 time.Time (24 each)
	return int64(8 + (16 + len(e.str)) + 8 + 24 + 24)
}

// GetFrequency returns the usage frequency for eviction scoring (implements eviction.Evictable).
func (e *tagEntry) GetFrequency() float64 {
	return float64(e.usageCount)
}

// GetCreatedAt returns when this tag was created (implements eviction.Evictable).
func (e *tagEntry) GetCreatedAt() time.Time {
	return e.createdAt
}

// GetLastAccessAt returns when this tag was last accessed (implements eviction.Evictable).
func (e *tagEntry) GetLastAccessAt() time.Time {
	return e.lastAccessAt
}
