// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestThreadKeyGroupsTurns: every turn of one conversation re-sends the same
// first user message, so all its turns map to the same thread key — while
// different first prompts (and different sessions) get distinct keys.
func TestThreadKeyGroupsTurns(t *testing.T) {
	turn1 := llmSpanInfo{
		sessionID: "trip-1",
		messages:  []llmMessage{{role: "system", content: "be helpful"}, {role: "user", content: "weather in Paris?"}},
	}
	// A follow-up turn carries the growing history but the same first user msg.
	turn2 := llmSpanInfo{
		sessionID: "trip-1",
		messages: []llmMessage{
			{role: "system", content: "be helpful"},
			{role: "user", content: "weather in Paris?"},
			{role: "assistant", content: "It's sunny."},
			{role: "user", content: "and tomorrow?"},
		},
	}
	assert.Equal(t, threadKey(turn1), threadKey(turn2), "turns of one conversation share a thread key")

	other := llmSpanInfo{sessionID: "trip-1", messages: []llmMessage{{role: "user", content: "weather in London?"}}}
	assert.NotEqual(t, threadKey(turn1), threadKey(other), "different first prompt -> different thread")

	otherSession := llmSpanInfo{sessionID: "trip-2", messages: []llmMessage{{role: "user", content: "weather in Paris?"}}}
	assert.NotEqual(t, threadKey(turn1), threadKey(otherSession), "same prompt, different session -> different thread")

	assert.Equal(t, "", threadKey(llmSpanInfo{}), "no session and no message -> not threadable")
}

// TestIsEmbeddingBody distinguishes an embeddings request (has "input", no
// "messages") from a chat request.
func TestIsEmbeddingBody(t *testing.T) {
	emb := []byte(`{"model":"text-embedding-3-small","input":"hello world"}`)
	chat := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`)

	assert.True(t, isEmbeddingBody(emb), "embeddings request")
	assert.False(t, isEmbeddingBody(chat), "chat request is not an embedding")
	assert.Equal(t, "hello world", parseEmbeddingInput(emb))
}
