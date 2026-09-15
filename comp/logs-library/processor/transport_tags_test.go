// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/agent-payload/v5/pb"

	"github.com/DataDog/datadog-agent/comp/logs-library/tagfilter"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// fakeTagFilter drops any tag whose key is in Drop, and records every
// argument it is called with. It intentionally does not depend on the real
// matcher, which is being implemented concurrently.
type fakeTagFilter struct {
	Drop         map[string]bool
	KeepCalls    [][]string
	RetainsCalls []string
}

func (f *fakeTagFilter) Keep(tags []string) []string {
	f.KeepCalls = append(f.KeepCalls, tags)
	out := make([]string, 0, len(tags))
	for _, t := range tags {
		key, _, _ := strings.Cut(t, ":")
		if f.Drop[key] {
			continue
		}
		out = append(out, t)
	}
	return out
}

func (f *fakeTagFilter) Retains(tag string) bool {
	f.RetainsCalls = append(f.RetainsCalls, tag)
	key, _, _ := strings.Cut(tag, ":")
	return !f.Drop[key]
}

// TestResolveTagFilter_TypedNilTrap pins the requirement that a source with
// nothing configured (neither global nor per-source rules) stamps a message
// with a genuinely nil sources.TagFilter interface, not a non-nil interface
// wrapping a nil *tagfilter.Scoped. Only "== nil" catches the regression;
// assert.Nil would pass either way.
func TestResolveTagFilter_TypedNilTrap(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage([]byte("hello"), source, message.StatusInfo)

	f := p.resolveTagFilter(msg)
	assert.True(t, f == nil, "expected a genuinely nil TagFilter interface, got %#v", f)

	resolved, ok := source.TagFilter()
	assert.True(t, ok, "source should be marked resolved after the first message")
	assert.True(t, resolved == nil, "expected a genuinely nil TagFilter interface, got %#v", resolved)
}

// TestResolveTagFilter_ResolvesOnceAndReuses asserts that once a source's
// filter has been resolved, later messages reuse the cached value instead of
// recomputing it. The sentinel is unreachable from a fresh compile (which
// would produce a nil filter in this test, given no rules are configured), so
// seeing it on msg2 proves resolveTagFilter took the "already resolved" path.
func TestResolveTagFilter_ResolvesOnceAndReuses(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{})

	msg1 := newMessage([]byte("one"), source, message.StatusInfo)
	p.resolveTagFilter(msg1)

	// Stamp a sentinel for the same generation p already resolved against, so a
	// second call can only see it by reusing the cache, never by recomputing.
	sentinel := &fakeTagFilter{Drop: map[string]bool{"sentinel": true}}
	source.CompareAndSwapTagFilterState(source.TagFilterState(), sources.NewTagFilterState(p.tagFilters, sentinel))

	msg2 := newMessage([]byte("two"), source, message.StatusInfo)
	assert.Same(t, sentinel, p.resolveTagFilter(msg2))
}

// TestResolveTagFilter_NoSource is a defensive regression: a message with no
// Origin/LogSource must not panic.
func TestResolveTagFilter_NoSource(t *testing.T) {
	p := &Processor{}
	msg := message.NewMessage([]byte("hello"), nil, message.StatusInfo, 0)
	var f sources.TagFilter
	assert.NotPanics(t, func() { f = p.resolveTagFilter(msg) })
	assert.True(t, f == nil)
}

// TestResolveTagFilter_MalformedPatternStillResolvesAndRecordsMessage pins the
// feature's failure mode: a malformed per-source pattern degrades to
// filtering less, it never stops the source from resolving (and so tailing
// and shipping), and it is surfaced on the source's status block.
func TestResolveTagFilter_MalformedPatternStillResolvesAndRecordsMessage(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"no_colon_here"}},
	})
	msg := newMessage([]byte("hello"), source, message.StatusInfo)

	assert.NotPanics(t, func() { p.resolveTagFilter(msg) })

	_, ok := source.TagFilter()
	assert.True(t, ok, "the source must still resolve despite the malformed pattern")

	found := false
	for _, m := range source.Messages.GetMessages() {
		if strings.Contains(m, "no_colon_here") {
			found = true
		}
	}
	assert.True(t, found, "expected a message naming the rejected pattern, got %v", source.Messages.GetMessages())
}

// TestResolveTagFilter_RegistersTagFilterInfoWhenConfigured asserts that a
// source with a valid pattern gets a "Tag Filters" status provider naming it.
func TestResolveTagFilter_RegistersTagFilterInfoWhenConfigured(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"container_id:*"}},
	})
	msg := newMessage([]byte("hello"), source, message.StatusInfo)

	p.resolveTagFilter(msg)

	info := source.GetInfo("Tag Filters")
	if assert.NotNil(t, info, "expected a Tag Filters info provider to be registered") {
		assert.Contains(t, strings.Join(info.Info(), " | "), "container_id:*")
	}
}

// TestResolveTagFilter_NoInfoProviderWhenUnconfigured asserts that an
// unconfigured source shows no Tag Filters block at all.
func TestResolveTagFilter_NoInfoProviderWhenUnconfigured(t *testing.T) {
	p := &Processor{}
	source := sources.NewLogSource("", &config.LogsConfig{})
	msg := newMessage([]byte("hello"), source, message.StatusInfo)

	p.resolveTagFilter(msg)

	assert.Nil(t, source.GetInfo("Tag Filters"))
}

// TestResolveSourceTagFilter_CalledTwice_RegistersOnce pins the idempotency the
// eager subscriber and the processor both rely on: racing or repeated calls for
// the same source must not double up its status info or messages. The state
// pointer identity check proves the second call took the early-return path
// rather than recomputing and overwriting with an equivalent value.
func TestResolveSourceTagFilter_CalledTwice_RegistersOnce(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"team:*"})
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"no_colon_here"}},
	})

	ResolveSourceTagFilter(global, source)
	stateAfterFirst := source.TagFilterState()
	ResolveSourceTagFilter(global, source)

	assert.Same(t, stateAfterFirst, source.TagFilterState(), "second call for the same global must not recompute")

	info := source.GetInfo("Tag Filters")
	if assert.NotNil(t, info) {
		assert.Equal(t, 1, strings.Count(strings.Join(info.Info(), "|"), "team:*"))
	}

	rejectedCount := 0
	for _, m := range source.Messages.GetMessages() {
		if strings.Contains(m, "no_colon_here") {
			rejectedCount++
		}
	}
	assert.Equal(t, 1, rejectedCount)
}

// TestResolveSourceTagFilter_NewGlobalForcesReResolution pins the generation
// check that lets a source surviving an agent restart detect that the global
// filter it was resolved against is stale: the same global pointer must reuse
// the cached state, and a different one must replace it.
func TestResolveSourceTagFilter_NewGlobalForcesReResolution(t *testing.T) {
	gen1, _ := tagfilter.Compile(nil, []string{"team:*"})
	gen2, _ := tagfilter.Compile(nil, []string{"pod_name:*"})
	source := sources.NewLogSource("", &config.LogsConfig{})

	ResolveSourceTagFilter(gen1, source)
	stateAfterGen1 := source.TagFilterState()

	ResolveSourceTagFilter(gen1, source)
	assert.Same(t, stateAfterGen1, source.TagFilterState(), "the same global must not force re-resolution")

	ResolveSourceTagFilter(gen2, source)
	assert.NotSame(t, stateAfterGen1, source.TagFilterState(), "a new global must force re-resolution")
	assert.True(t, source.TagFilterState().ResolvedFor(gen2))
}

// TestResolveSourceTagFilter_SourceOnlyStillGetsInfo asserts that a source with
// per-source filters but no global block still renders a status block.
func TestResolveSourceTagFilter_SourceOnlyStillGetsInfo(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"container_id:*"}},
	})

	ResolveSourceTagFilter(nil, source)

	info := source.GetInfo("Tag Filters")
	if assert.NotNil(t, info) {
		assert.Contains(t, strings.Join(info.Info(), " | "), "source exclude: container_id:*")
	}
}

// TestResolveSourceTagFilter_NeitherScopeConfigured_NoInfo asserts that a source
// with no filters configured anywhere gets no status block.
func TestResolveSourceTagFilter_NeitherScopeConfigured_NoInfo(t *testing.T) {
	source := sources.NewLogSource("", &config.LogsConfig{})

	ResolveSourceTagFilter(nil, source)

	assert.Nil(t, source.GetInfo("Tag Filters"))
	resolved, ok := source.TagFilter()
	assert.True(t, ok)
	assert.True(t, resolved == nil)
}

// TestResolveSourceTagFilter_ConcurrentCallsConverge exercises the race between
// the eager subscriber and the processor resolving the same source. Run with
// -race to check for unsynchronized access.
func TestResolveSourceTagFilter_ConcurrentCallsConverge(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"team:*"})
	source := sources.NewLogSource("", &config.LogsConfig{
		TagFilters: &config.TagFilters{Exclude: []string{"pod_name:*"}},
	})

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ResolveSourceTagFilter(global, source)
		}()
	}
	wg.Wait()

	resolved, ok := source.TagFilter()
	assert.True(t, ok)
	assert.NotNil(t, resolved)
	assert.NotNil(t, source.GetInfo("Tag Filters"))
}

// TestByteParity_NilFilterMatchesUnfilteredAccessors is the most important
// test in the set: with no filter stamped, every encoder must produce output
// byte-identical to the untouched, unfiltered accessors.
func TestByteParity_NilFilterMatchesUnfilteredAccessors(t *testing.T) {
	logsConfig := &config.LogsConfig{
		Service:        "Service",
		Source:         "Source",
		SourceCategory: "SourceCategory",
		Tags:           []string{"foo:bar", "baz"},
	}
	source := sources.NewLogSource("", logsConfig)

	buildMsg := func() *message.Message {
		msg := newMessage([]byte("message"), source, message.StatusInfo)
		msg.State = message.StateRendered
		msg.Origin.LogSource = source
		msg.Origin.SetTags([]string{"a", "b:c"})
		return msg
	}

	t.Run("json", func(t *testing.T) {
		msg := buildMsg()
		expected := msg.TagsToString()
		assert.NoError(t, JSONEncoder.Encode(msg, "host", nil))

		var decoded jsonPayload
		assert.NoError(t, json.Unmarshal(msg.GetContent(), &decoded))
		assert.Equal(t, expected, decoded.Tags)
	})

	t.Run("proto", func(t *testing.T) {
		msg := buildMsg()
		expected := msg.Tags()
		assert.NoError(t, ProtoEncoder.Encode(msg, "host", nil))

		log := &pb.Log{}
		assert.NoError(t, log.Unmarshal(msg.GetContent()))
		assert.Equal(t, expected, log.Tags)
	})

	t.Run("raw", func(t *testing.T) {
		msg := buildMsg()
		expected := msg.Origin.TagsPayload(nil)
		assert.NoError(t, RawEncoder.Encode(msg, "host", nil))

		content := string(msg.GetContent())
		extra := content[strings.Index(content, "[") : strings.LastIndex(content, "]")+1]
		assert.Equal(t, string(expected), extra)
	})
}

// TestEncoders_ApplyStampedFilter checks each encoder against a stamped
// filter: excluded tags are absent, kept tags survive, ddsource is never
// dropped, and ddsourcecategory is dropped via a single Retains check.
func TestEncoders_ApplyStampedFilter(t *testing.T) {
	logsConfig := &config.LogsConfig{
		Source:         "mysource",
		SourceCategory: "cat",
		Tags:           []string{"keep:me", "drop:me"},
	}
	source := sources.NewLogSource("", logsConfig)
	f := &fakeTagFilter{Drop: map[string]bool{"drop": true, "sourcecategory": true, "source": true}}

	buildMsg := func() *message.Message {
		msg := newMessage([]byte("message"), source, message.StatusInfo)
		msg.State = message.StateRendered
		msg.Origin.LogSource = source
		return msg
	}

	t.Run("json", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, JSONEncoder.Encode(msg, "host", f))
		var decoded jsonPayload
		assert.NoError(t, json.Unmarshal(msg.GetContent(), &decoded))
		assert.NotContains(t, decoded.Tags, "drop:me")
		assert.Contains(t, decoded.Tags, "keep:me")
	})

	t.Run("proto", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, ProtoEncoder.Encode(msg, "host", f))
		log := &pb.Log{}
		assert.NoError(t, log.Unmarshal(msg.GetContent()))
		assert.NotContains(t, log.Tags, "drop:me")
		assert.Contains(t, log.Tags, "keep:me")
	})

	t.Run("raw", func(t *testing.T) {
		msg := buildMsg()
		assert.NoError(t, RawEncoder.Encode(msg, "host", f))
		content := string(msg.GetContent())
		// ddsource survives even though the filter would drop the "source" key.
		assert.Contains(t, content, "ddsource=\"mysource\"")
		// ddsourcecategory is dropped via a single Retains("sourcecategory:cat") check.
		assert.NotContains(t, content, "ddsourcecategory")
		assert.NotContains(t, content, "drop:me")
		assert.Contains(t, content, "keep:me")
	})
}

// TestResolveSourceTagFilter_NilConfigDoesNotPanic pins the replay path:
// LogSources.AddSource appends a source to its slice before rejecting it for a
// nil Config, and SubscribeAll replays that slice, so the eager subscriber can
// be handed a source with no Config at all.
func TestResolveSourceTagFilter_NilConfigDoesNotPanic(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"container_id:*"})

	assert.NotPanics(t, func() {
		ResolveSourceTagFilter(global, sources.NewLogSource("nil-config", nil))
	})
	assert.NotPanics(t, func() {
		ResolveSourceTagFilter(global, nil)
	})
}
