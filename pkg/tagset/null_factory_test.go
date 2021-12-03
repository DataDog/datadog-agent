// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// NOTE: this file is named *_test.go because it is only intended for use in
// tests within this package.

package tagset

import (
	"strings"
	"testing"

	"github.com/twmb/murmur3"
)

func TestNullFactory(t *testing.T) {
	testFactory(t, func() Factory { return newNullFactory() })
}

// A nullFactory caches nothing. It is useful for tests that need a factory.
type nullFactory struct {
	baseFactory
}

var _ Factory = (*nullFactory)(nil)

func newNullFactory() *nullFactory {
	return &nullFactory{}
}

// NewTags implements Factory.NewTags
func (f *nullFactory) NewTags(tags []string) *Tags {
	tagsMap := make(map[string]struct{}, len(tags))
	for _, t := range tags {
		tagsMap[t] = struct{}{}
	}
	return f.NewTagsFromMap(tagsMap)
}

// NewUniqueTags implements Factory.NewUniqueTags
func (f *nullFactory) NewUniqueTags(tags ...string) *Tags {
	hashes, hash := calcHashes(tags)
	return &Tags{tags, hashes, hash}
}

// NewTagsFromMap implements Factory.NewTagsFromMap
func (f *nullFactory) NewTagsFromMap(src map[string]struct{}) *Tags {
	tags := make([]string, 0, len(src))
	for tag := range src {
		tags = append(tags, tag)
	}
	hashes, hash := calcHashes(tags)
	return &Tags{tags, hashes, hash}
}

// NewTag implements Factory.NewTag
func (f *nullFactory) NewTag(tag string) *Tags {
	hash := murmur3.StringSum64(tag)
	tags := []string{tag}
	hashes := []uint64{hash}
	return &Tags{tags, hashes, hash}
}

// NewBuilder implements Factory.NewBuilder
func (f *nullFactory) NewBuilder(capacity int) *Builder {
	return f.baseFactory.newBuilder(f, capacity)
}

// NewBuilder implements Factory.NewBuilder
func (f *nullFactory) NewSliceBuilder(levels, capacity int) *SliceBuilder {
	return f.baseFactory.newSliceBuilder(f, levels, capacity)
}

// ParseDSD implements Factory.ParseDSD
func (f *nullFactory) ParseDSD(data []byte) (*Tags, error) {
	tags := strings.Split(string(data), ",")
	return f.NewTags(tags), nil
}

// Union implements Factory.Union
func (f *nullFactory) Union(a, b *Tags) *Tags {
	tags := make(map[string]struct{}, len(a.tags)+len(b.tags))
	for _, t := range a.tags {
		tags[t] = struct{}{}
	}
	for _, t := range b.tags {
		tags[t] = struct{}{}
	}
	slice := make([]string, 0, len(tags))
	for tag := range tags {
		slice = append(slice, tag)
	}
	return f.NewTagsFromMap(tags)
}

// UnsafeDisjointUnion implements Factory.DisjoingUnion
func (f *nullFactory) UnsafeDisjointUnion(a, b *Tags) *Tags {
	tags := make([]string, len(a.tags)+len(b.tags))
	copy(tags[:len(a.tags)], a.tags)
	copy(tags[len(a.tags):], b.tags)

	hashes := make([]uint64, len(a.hashes)+len(b.hashes))
	copy(hashes[:len(a.hashes)], a.hashes)
	copy(hashes[len(a.hashes):], b.hashes)

	hash := a.hash ^ b.hash

	return &Tags{tags, hashes, hash}
}

// getCachedTags implements Factory.getCachedTags
func (f *nullFactory) getCachedTags(cacheID cacheID, key uint64, miss func() *Tags) *Tags {
	return miss()
}

// getCachedTagsErr implements Factory.getCachedTagsErr
func (f *nullFactory) getCachedTagsErr(cacheID cacheID, key uint64, miss func() (*Tags, error)) (*Tags, error) {
	return miss()
}
