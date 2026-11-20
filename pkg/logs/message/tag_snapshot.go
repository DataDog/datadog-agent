// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package message

import (
	"iter"
	"slices"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TagSnapshot is an immutable set of tags shared by consecutive messages from one producer.
// A changed tag set always gets a new snapshot, so pointer identity identifies its version.
type TagSnapshot struct {
	config         *config.LogsConfig
	configTags     []string
	sourceCategory string
	service        string
	static         []string
	dynamic        []string

	cache atomic.Pointer[any]
}

func newTagSnapshot(source *sources.LogSource, static, dynamic []string) *TagSnapshot {
	return &TagSnapshot{
		config:         source.Config,
		configTags:     source.Config.Tags,
		sourceCategory: source.Config.SourceCategory,
		service:        source.Config.Service,
		static:         static,
		dynamic:        dynamic,
	}
}

// All yields every tag Origin.HasTag would match through this snapshot.
func (s *TagSnapshot) All() iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, group := range [][]string{s.static, s.dynamic, s.configTags} {
			for _, t := range group {
				if !yield(t) {
					return
				}
			}
		}
		if s.sourceCategory != "" && !yield("sourcecategory:"+s.sourceCategory) {
			return
		}
		if s.service != "" {
			yield("service:" + s.service)
		}
	}
}

// Cached returns the value last stored with SetCached, or nil.
func (s *TagSnapshot) Cached() any {
	if v := s.cache.Load(); v != nil {
		return *v
	}
	return nil
}

// SetCached attaches a value derived from this snapshot, such as a precomputed match plan.
func (s *TagSnapshot) SetCached(v any) {
	s.cache.Store(&v)
}

func (s *TagSnapshot) describes(source *sources.LogSource) bool {
	c := source.Config
	return c == s.config && sameSlice(c.Tags, s.configTags) && c.SourceCategory == s.sourceCategory && c.Service == s.service
}

// TagSnapshotter returns the previous snapshot while a producer's shared tags are unchanged.
// It is not safe for concurrent use.
type TagSnapshotter struct {
	last *TagSnapshot
}

// Get returns a snapshot of source's config tags plus static and dynamic, or nil without a source config.
// static is compared by identity and dynamic by content; neither may be modified afterwards.
func (s *TagSnapshotter) Get(source *sources.LogSource, static, dynamic []string) *TagSnapshot {
	if source == nil || source.Config == nil {
		return nil
	}
	if l := s.last; l != nil && l.describes(source) && sameSlice(l.static, static) && slices.Equal(l.dynamic, dynamic) {
		return l
	}
	s.last = newTagSnapshot(source, static, dynamic)
	return s.last
}

func sameSlice(a, b []string) bool {
	return len(a) == len(b) && (len(a) == 0 || &a[0] == &b[0])
}
