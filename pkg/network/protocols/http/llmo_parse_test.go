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

// These tests exercise the typed JSON parser on cases the tolerant regex
// extractor gets wrong: content containing escaped quotes (the regex's
// "content":"([^"]*)" stops at the first escaped quote), unicode escapes, and
// token usage read from a complete response object.

func TestParseLLMBodyJSONEscaping(t *testing.T) {
	tests := []struct {
		name       string
		raw        []byte
		wantModel  string
		wantPrompt string
	}{
		{
			name:       "content with escaped quotes",
			raw:        []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"say \"hi\" to Bob"}]}`),
			wantModel:  "gpt-4o-mini",
			wantPrompt: `say "hi" to Bob`,
		},
		{
			name:       "content with unicode escape",
			raw:        []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"café au lait"}]}`),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "café au lait",
		},
		{
			name:       "behind http2 frame header, nul padded",
			raw:        padTo(append(http2DataFrameHeader(60), []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi, there"}]}`)...), llmBodyBufferSize),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "hi, there",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model, prompt := parseLLMBody(tt.raw)
			assert.Equal(t, tt.wantModel, model, "model")
			assert.Equal(t, tt.wantPrompt, prompt, "prompt")
		})
	}
}

func TestParseLLMMessagesJSONEscaping(t *testing.T) {
	// The system message contains escaped quotes; the user message contains a
	// comma. Both are recovered exactly by the JSON decoder.
	raw := []byte(`{"model":"gpt-4o-mini","messages":[` +
		`{"role":"system","content":"Always use \"tools\"."},` +
		`{"role":"user","content":"weather in Paris, France?"}]}`)
	got := parseLLMMessages(raw, providerOpenAI)
	assert.Equal(t, []llmMessage{
		{role: "system", content: `Always use "tools".`},
		{role: "user", content: "weather in Paris, France?"},
	}, got)
}

func TestParseResponseTextJSONEscaping(t *testing.T) {
	raw := []byte(`{"choices":[{"message":{"role":"assistant","content":"He replied: \"done\"."}}]}`)
	assert.Equal(t, `He replied: "done".`, parseResponseText(raw, providerOpenAI))

	anthropic := []byte(`{"content":[{"type":"text","text":"quote: \"ok\""}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2}}`)
	assert.Equal(t, `quote: "ok"`, parseResponseText(anthropic, providerAnthropic))
}

func TestParseFullResponseJSON(t *testing.T) {
	// A complete OpenAI response object (not a head/tail fragment): tool calls,
	// usage, and the tool-call-generation flag all decode from JSON.
	raw := []byte(`{"id":"chatcmpl-1","object":"chat.completion",` +
		`"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[` +
		`{"id":"call_x","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris, France\"}"}}]},` +
		`"finish_reason":"tool_calls"}],` +
		`"usage":{"prompt_tokens":11,"completion_tokens":7,"total_tokens":18}}`)

	assert.Equal(t, []llmToolCall{{id: "call_x", name: "get_weather", arguments: `{"city":"Paris, France"}`}},
		parseToolCalls(raw, providerOpenAI))

	in, out, tot := parseLLMUsage(raw, providerOpenAI)
	assert.Equal(t, int64(11), in, "input_tokens")
	assert.Equal(t, int64(7), out, "output_tokens")
	assert.Equal(t, int64(18), tot, "total_tokens")

	assert.True(t, isToolCallGen(raw, providerOpenAI), "finish_reason=tool_calls")

	// A plain-text full response is not a tool-call generation.
	plain := []byte(`{"choices":[{"message":{"role":"assistant","content":"the weather is sunny"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`)
	assert.False(t, isToolCallGen(plain, providerOpenAI))
	assert.Equal(t, "the weather is sunny", parseResponseText(plain, providerOpenAI))
}

func TestParseToolResultsJSONEscaping(t *testing.T) {
	// Anthropic tool_result content with a comma, quotes, and a non-ASCII char.
	raw := []byte(`{"model":"claude-haiku-4-5","messages":[` +
		`{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_9","content":"18°C, \"sunny\""}]}]}`)
	assert.Equal(t, []llmToolResult{{id: "toolu_9", content: `18°C, "sunny"`}},
		parseToolResults(raw, providerAnthropic))
}

func TestParseFullResponseFallsBackOnFragment(t *testing.T) {
	// A response *tail* fragment (the usage object is nested, no choices) must
	// fall back to the regex extractor rather than mis-decoding the inner object.
	tail := []byte(`"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":40,"completion_tokens":12,"total_tokens":52}}`)
	in, out, tot := parseLLMUsage(tail, providerOpenAI)
	assert.Equal(t, int64(40), in)
	assert.Equal(t, int64(12), out)
	assert.Equal(t, int64(52), tot)
	assert.True(t, isToolCallGen(tail, providerOpenAI), "regex fallback finds finish_reason on the fragment")
}
