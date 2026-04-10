// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package grpc

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	rtokenizer "github.com/DataDog/datadog-agent/pkg/logs/patterns/tokenizer/rust"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// makeTestOrigin builds an Origin backed by a LogsConfig with explicit service/source.
func makeTestOrigin(service, source string, tags []string) *message.Origin {
	logSource := sources.NewLogSource("test-source", &config.LogsConfig{
		Type:    "file",
		Path:    "/var/log/test.log",
		Service: service,
		Source:  source,
	})
	origin := message.NewOrigin(logSource)
	origin.SetTags(tags)
	return origin
}

// makeMsg creates a Message with the given content, origin, hostname, and status.
func makeMsg(content, hostname, status string, origin *message.Origin) *message.Message {
	msg := message.NewMessage([]byte(content), origin, status, time.Now().UnixNano())
	msg.Hostname = hostname
	return msg
}

func TestBuildTagSet_CacheCorrectness(t *testing.T) {
	tok := rtokenizer.NewRustTokenizer()
	mt := NewMessageTranslator("test-pipeline", tok)

	origin := makeTestOrigin("svc-a", "src-a", []string{"env:test"})

	// ── Case 1: initial call populates cache ──────────────────────────────────
	msg1 := makeMsg("log line 1", "host-1", "info", origin)
	tagSet1, tagStr1, dictID1, isNew1 := mt.buildTagSet(msg1)

	require.NotNil(t, tagSet1, "first call must return a non-nil TagSet")
	assert.True(t, isNew1, "first call must create a new dict entry")
	assert.NotEmpty(t, tagStr1, "allTagsString must be non-empty")
	assert.Contains(t, tagStr1, "hostname:host-1")
	assert.Contains(t, tagStr1, "service:svc-a")
	assert.Contains(t, tagStr1, "ddsource:src-a")

	// Verify cache is populated (internal state)
	assert.Equal(t, tagSet1, mt.tagCache.tagSet)
	assert.Equal(t, tagStr1, mt.tagCache.tagStr)
	assert.Equal(t, dictID1, mt.tagCache.dictID)

	// ── Case 2: cache hit — same inputs → isNew=false, same allTagsString ─────
	msg2 := makeMsg("log line 2", "host-1", "info", origin)
	tagSet2, tagStr2, dictID2, isNew2 := mt.buildTagSet(msg2)

	assert.False(t, isNew2, "repeated identical call must be a cache hit (isNew=false)")
	assert.Equal(t, tagStr1, tagStr2, "cache hit must return same allTagsString")
	assert.Equal(t, dictID1, dictID2, "cache hit must return same dictID")
	assert.Equal(t, tagSet1, tagSet2, "cache hit must return same *TagSet pointer")

	// ── Case 3: hostname change causes cache miss ─────────────────────────────
	msg3 := makeMsg("log line 3", "host-2", "info", origin)
	tagSet3, tagStr3, _, isNew3 := mt.buildTagSet(msg3)

	require.NotNil(t, tagSet3)
	// isNew3 may be true (new dict entry) — at minimum the tag string must differ
	_ = isNew3
	assert.Contains(t, tagStr3, "hostname:host-2", "new hostname must appear in tag string")
	assert.NotEqual(t, tagStr1, tagStr3, "hostname change must produce a different allTagsString")
	// Cache must now reflect the new hostname
	assert.Equal(t, "host-2", mt.tagCache.hostname)

	// ── Case 4: service change causes cache miss ──────────────────────────────
	origin2 := makeTestOrigin("svc-b", "src-a", []string{"env:test"})
	msg4 := makeMsg("log line 4", "host-2", "info", origin2)
	tagSet4, tagStr4, _, _ := mt.buildTagSet(msg4)

	require.NotNil(t, tagSet4)
	assert.Contains(t, tagStr4, "service:svc-b", "new service must appear in tag string")
	assert.NotContains(t, tagStr4, "service:svc-a", "old service must not appear after service change")
	assert.NotEqual(t, tagStr3, tagStr4, "service change must produce a different allTagsString")

	// ── Case 5: source change causes cache miss ───────────────────────────────
	origin3 := makeTestOrigin("svc-b", "src-b", []string{"env:test"})
	msg5 := makeMsg("log line 5", "host-2", "info", origin3)
	tagSet5, tagStr5, _, _ := mt.buildTagSet(msg5)

	require.NotNil(t, tagSet5)
	assert.Contains(t, tagStr5, "ddsource:src-b", "new source must appear in tag string")
	assert.NotContains(t, tagStr5, "ddsource:src-a", "old source must not appear after source change")
	assert.NotEqual(t, tagStr4, tagStr5, "source change must produce a different allTagsString")

	// ── Case 6: status change causes cache miss ───────────────────────────────
	// Use the same origin3 (same origin ptr, service, source) but change status
	msg6 := makeMsg("log line 6", "host-2", "error", origin3)
	tagSet6, tagStr6, _, _ := mt.buildTagSet(msg6)

	require.NotNil(t, tagSet6)
	assert.Contains(t, tagStr6, "status:error", "new status must appear in tag string")
	// "info" is the default status (returned by GetStatus when Status=="") so it appears
	// in tagStr5. After changing to "error" the strings must differ.
	assert.True(t,
		strings.Contains(tagStr5, "status:info") || !strings.Contains(tagStr5, "status:error"),
		"prior tag string must not already contain status:error")
	assert.NotEqual(t, tagStr5, tagStr6, "status change must produce a different allTagsString")
	assert.Equal(t, "error", mt.tagCache.status, "cache must reflect updated status")
}

func TestBuildTagSet_ProcessingTagsInvalidateCache(t *testing.T) {
	tok := rtokenizer.NewRustTokenizer()
	mt := NewMessageTranslator("test-pipeline", tok)

	origin := makeTestOrigin("svc-a", "src-a", []string{"env:test"})

	msg1 := makeMsg("log line 1", "host-1", "info", origin)
	msg1.ProcessingTags = []string{"team:red"}
	tagSet1, tagStr1, dictID1, isNew1 := mt.buildTagSet(msg1)

	require.NotNil(t, tagSet1)
	require.True(t, isNew1)
	assert.Contains(t, tagStr1, "team:red")
	assert.NotContains(t, tagStr1, "team:blue")

	msg2 := makeMsg("log line 2", "host-1", "info", origin)
	msg2.ProcessingTags = []string{"team:blue"}
	tagSet2, tagStr2, dictID2, isNew2 := mt.buildTagSet(msg2)

	require.NotNil(t, tagSet2)
	assert.True(t, isNew2, "different processing tags should build a new tagset string")
	assert.NotEqual(t, dictID1, dictID2, "different processing tags should use a different dictionary entry")
	assert.NotEqual(t, tagSet1, tagSet2, "different processing tags should not reuse the cached TagSet")
	assert.Contains(t, tagStr2, "team:blue")
	assert.NotContains(t, tagStr2, "team:red")
	assert.NotEqual(t, tagStr1, tagStr2, "different processing tags should change the final joined tag string")
}
