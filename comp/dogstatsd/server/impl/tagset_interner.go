// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverimpl

import (
	"os"
	"strconv"
	"strings"
)

func parserTagsetInternerEnabled() bool {
	enabled, err := strconv.ParseBool(os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER"))
	return err == nil && enabled
}

func parserTagsetInternerSize(defaultSize int) int {
	if raw := os.Getenv("DD_DOGSTATSD_EXPERIMENTAL_PARSE_TAGSET_INTERNER_SIZE"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err == nil {
			return value
		}
	}
	if defaultSize > 0 {
		return defaultSize
	}
	return 4096
}

type parserTagsetInterner struct {
	maxSize int
	tagsets map[string][]string
	ring    []string
	next    int

	seen     map[uint64]struct{}
	seenRing []uint64
	seenNext int

	hits               uint64
	missNotAdmitted    uint64
	specialNotAdmitted uint64
	admitted           uint64
	evictions          uint64
}

func newParserTagsetInterner(defaultSize int) *parserTagsetInterner {
	if !parserTagsetInternerEnabled() {
		return nil
	}
	maxSize := parserTagsetInternerSize(defaultSize)
	if maxSize <= 0 {
		return nil
	}
	seenSize := maxSize * 4
	if seenSize < 1024 {
		seenSize = 1024
	}
	return &parserTagsetInterner{
		maxSize:  maxSize,
		tagsets:  make(map[string][]string),
		ring:     make([]string, maxSize),
		seen:     make(map[uint64]struct{}),
		seenRing: make([]uint64, seenSize),
	}
}

func (i *parserTagsetInterner) LoadOrParse(rawTags []byte, parse func([]byte) []string) []string {
	if i == nil || len(rawTags) == 0 {
		return parse(rawTags)
	}

	// Keep string(rawTags) directly in the lookup expression so cache hits do
	// not allocate a temporary string.
	if tags, found := i.tagsets[string(rawTags)]; found {
		i.hits++
		return tags
	}

	hash := hashBytes64(rawTags)
	_, seen := i.seen[hash]
	if !seen {
		i.recordSeen(hash)
		i.missNotAdmitted++
		return parse(rawTags)
	}

	tags := parse(rawTags)
	if hasParserSpecialTags(tags) {
		i.specialNotAdmitted++
		return tags
	}

	key := string(rawTags)
	i.insert(key, tags)
	i.admitted++
	return tags
}

func (i *parserTagsetInterner) recordSeen(hash uint64) {
	if len(i.seenRing) == 0 {
		return
	}
	evicted := i.seenRing[i.seenNext]
	if evicted != 0 {
		delete(i.seen, evicted)
	}
	i.seenRing[i.seenNext] = hash
	i.seen[hash] = struct{}{}
	i.seenNext = (i.seenNext + 1) % len(i.seenRing)
}

func (i *parserTagsetInterner) insert(key string, tags []string) {
	if _, found := i.tagsets[key]; found {
		return
	}
	evicted := i.ring[i.next]
	if evicted != "" {
		if _, found := i.tagsets[evicted]; found {
			delete(i.tagsets, evicted)
			i.evictions++
		}
	}
	i.ring[i.next] = key
	i.next = (i.next + 1) % len(i.ring)
	i.tagsets[key] = tags
}

func hasParserSpecialTags(tags []string) bool {
	for _, tag := range tags {
		if strings.HasPrefix(tag, hostTagPrefix) ||
			strings.HasPrefix(tag, entityIDTagPrefix) ||
			strings.HasPrefix(tag, CardinalityTagPrefix) ||
			strings.HasPrefix(tag, jmxCheckNamePrefix) {
			return true
		}
	}
	return false
}

func hashBytes64(b []byte) uint64 {
	const offset64 = 14695981039346656037
	const prime64 = 1099511628211
	h := uint64(offset64)
	for _, c := range b {
		h ^= uint64(c)
		h *= prime64
	}
	if h == 0 {
		return 1
	}
	return h
}
