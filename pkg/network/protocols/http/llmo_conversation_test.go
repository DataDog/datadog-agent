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

// TestGenUsagePairsByToolCallID is the regression test for bug #4: two tool
// workflows run concurrently on one connection. Turn-1 token usage must attach
// to the right workflow's first llm span — keyed by tool_call id, not by
// connection (a connection key would let workflow B's turn-1 overwrite A's, so
// A's follow-up would report B's cost).
func TestGenUsagePairsByToolCallID(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	conn := llmConnKey{SrcPort: 9090, DstPort: 443}

	genResp := func(id, total string) []byte {
		return []byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[` +
			`{"id":"` + id + `","type":"function","function":{"name":"get_x","arguments":"{}"}}]},` +
			`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":` + total + `}}`)
	}
	answer := []byte(`{"choices":[{"message":{"content":"final"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":3}}`)
	followup := func(id, prompt string) llmReqParsed {
		return llmReqParsed{
			provider:     providerOpenAI,
			prompt:       prompt,
			messages:     []llmMessage{{role: "user", content: prompt}},
			reqToolCalls: []llmToolCall{{id: id, name: "get_x", arguments: "{}"}},
			toolResults:  []llmToolResult{{id: id, content: "res"}},
		}
	}

	// Interleaved turn-1s (A usage 10, B usage 20) — with the old connection key,
	// B's would overwrite A's — then each workflow's follow-up.
	h.pairAndEmit(llmStreamKey{conn: conn, stream: 1}, llmReqParsed{provider: providerOpenAI, prompt: "A"}, genResp("call_A", "10"))
	h.pairAndEmit(llmStreamKey{conn: conn, stream: 3}, llmReqParsed{provider: providerOpenAI, prompt: "B"}, genResp("call_B", "20"))
	h.pairAndEmit(llmStreamKey{conn: conn, stream: 5}, followup("call_A", "A"), answer)
	h.pairAndEmit(llmStreamKey{conn: conn, stream: 7}, followup("call_B", "B"), answer)

	byPrompt := map[string]llmSpanInfo{}
	for _, e := range emitted {
		if len(e.reqToolCalls) > 0 { // the follow-up (workflow) emits
			byPrompt[e.prompt] = e
		}
	}
	assert.Equal(t, int64(10), byPrompt["A"].firstGenUsage.total, "workflow A keeps its own turn-1 usage")
	assert.Equal(t, int64(20), byPrompt["B"].firstGenUsage.total, "workflow B keeps its own turn-1 usage")
}
