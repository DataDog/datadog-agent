// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package collectors

import "errors"

// TagInfo holds the tag information for a given entity and source. It's meant
// to be created from collectors and read by the store.
type TagInfo struct {
	Source       string   // source collector's name
	Entity       string   // entity name ready for lookup
	HighCardTags []string // high cardinality tags that can create a lot of contexts
	LowCardTags  []string // low cardinality tags safe for every pipeline
	DeleteEntity bool     // true if the entity is to be deleted from the store
}

// CollectionMode informs the Tagger of how to schedule a Collector
type CollectionMode int

// ErrNotFound is returned by Fetch not collection error occurred but
// the entity is not found in the source
var ErrNotFound = errors.New("entity not found")

// Return values for Collector.Init to inform the Tagger of the scheduling needed
const (
	NoCollection        CollectionMode = iota // Not available
	PullCollection                            // Call regularly via the Pull method
	StreamCollection                          // Will continuously feed updates on the channel from Steam() to Stop()
	FetchOnlyCollection                       // Only call Fetch() on cache misses

)

// Collector retrieve entity tags from a given source and feeds
// updates via the TagInfo channel
type Collector interface {
	Detect(chan<- []*TagInfo) (CollectionMode, error)
}

// Fetcher allows to fetch tags on-demand in case of cache miss
type Fetcher interface {
	Fetch(string) ([]string, []string, error)
}

// Streamer feeds back TagInfo when detecting changes
type Streamer interface {
	Fetcher
	Stream() error
	Stop() error
}

// Puller has to be triggered regularly
type Puller interface {
	Fetcher
	Pull() error
}
