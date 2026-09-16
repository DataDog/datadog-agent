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

// fakeTagFilter isolates transport wiring from matcher behavior.
type fakeTagFilter struct {
	Drop map[string]bool
}

func (f *fakeTagFilter) Keep(tags []string) []string {
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
func (f *fakeTagFilter) RetainsTag(key, _ string) bool {
	return !f.Drop[key]
}

// TestResolveTagFilter_TypedNilTrap guards against an interface containing a
// nil *tagfilter.Scoped, which would compare non-nil.
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

// TestResolveTagFilter_MalformedPatternStillResolvesAndRecordsMessage verifies
// malformed rules degrade safely and remain visible in status.
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

// TestResolveSourceTagFilter_ConcurrentCallsConverge covers simultaneous eager
// and processor resolution.
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

// TestByteParity_NilFilterMatchesUnfilteredAccessors protects the no-filter
// encoder output.
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

// TestEncoders_ApplyStampedFilter covers filtered JSON, protobuf, and raw output.
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
		assert.Contains(t, content, "ddsource=\"mysource\"")
		assert.NotContains(t, content, "ddsourcecategory")
		assert.NotContains(t, content, "drop:me")
		assert.Contains(t, content, "keep:me")
	})
}

// TestResolveSourceTagFilter_NilConfigDoesNotPanic covers invalid sources
// replayed by SubscribeAll.
func TestResolveSourceTagFilter_NilConfigDoesNotPanic(t *testing.T) {
	global, _ := tagfilter.Compile(nil, []string{"container_id:*"})

	assert.NotPanics(t, func() {
		ResolveSourceTagFilter(global, sources.NewLogSource("nil-config", nil))
	})
	assert.NotPanics(t, func() {
		ResolveSourceTagFilter(global, nil)
	})
}
