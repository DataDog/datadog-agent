// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.Datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tagset

import "sync"

// threadSafeFactory wraps another factory and uses a mutex to control
// access.
type threadSafeFactory struct {
	// baseFactory implements newBuilder/builderClosed,
	// newSliceBuilder/sliceBuilderClosed for us
	baseFactory

	// mu synchronizes access to all fields
	mu sync.Mutex

	// inner is the embedded factory this instance synchronizes
	inner Factory
}

var _ Factory = (*threadSafeFactory)(nil)

// NewThreadSafeFactory wraps the given factory with a mutex, ensuring
// thread-safe operation.
func NewThreadSafeFactory(inner Factory) Factory {
	return &threadSafeFactory{inner: inner}
}

// NewTags implements Factory.NewTags
func (f *threadSafeFactory) NewTags(src []string) *Tags {
	f.mu.Lock()
	tags := f.inner.NewTags(src)
	f.mu.Unlock()
	return tags
}

// NewUniqueTags implements Factory.NewUniqueTags
func (f *threadSafeFactory) NewUniqueTags(src ...string) *Tags {
	f.mu.Lock()
	tags := f.inner.NewUniqueTags(src...)
	f.mu.Unlock()
	return tags
}

// NewTagsFromMap implements Factory.NewTagsFromMap
func (f *threadSafeFactory) NewTagsFromMap(src map[string]struct{}) *Tags {
	f.mu.Lock()
	tags := f.inner.NewTagsFromMap(src)
	f.mu.Unlock()
	return tags
}

// NewTag implements Factory.NewTag
func (f *threadSafeFactory) NewTag(tag string) *Tags {
	f.mu.Lock()
	tags := f.inner.NewTag(tag)
	f.mu.Unlock()
	return tags
}

// NewBuilder implements Factory.NewBuilder
func (f *threadSafeFactory) NewBuilder(capacity int) *Builder {
	f.mu.Lock()
	bldr := f.baseFactory.newBuilder(f, capacity)
	f.mu.Unlock()
	return bldr
}

// NewSliceBuilder implements Factory.NewSliceBuilder
func (f *threadSafeFactory) NewSliceBuilder(levels, capacity int) *SliceBuilder {
	f.mu.Lock()
	bldr := f.baseFactory.newSliceBuilder(f, levels, capacity)
	f.mu.Unlock()
	return bldr
}

// Union implements Factory.Union
func (f *threadSafeFactory) Union(a, b *Tags) *Tags {
	f.mu.Lock()
	tags := f.inner.Union(a, b)
	f.mu.Unlock()
	return tags
}

// getCachedTags implements Factory.getCachedTags
func (f *threadSafeFactory) getCachedTags(cacheID cacheID, key uint64, miss func() *Tags) *Tags {
	f.mu.Lock()
	tags := f.inner.getCachedTags(cacheID, key, miss)
	f.mu.Unlock()
	return tags
}

// getCachedTags implements Factory.getCachedTags
func (f *threadSafeFactory) getCachedTagsErr(cacheID cacheID, key uint64, miss func() (*Tags, error)) (*Tags, error) {
	f.mu.Lock()
	tags, err := f.inner.getCachedTagsErr(cacheID, key, miss)
	f.mu.Unlock()
	return tags, err
}

func (f *threadSafeFactory) builderClosed(builder *Builder) {
	f.mu.Lock()
	f.baseFactory.builderClosed(builder)
	f.mu.Unlock()
}

func (f *threadSafeFactory) sliceBuilderClosed(builder *SliceBuilder) {
	f.mu.Lock()
	f.baseFactory.sliceBuilderClosed(builder)
	f.mu.Unlock()
}
