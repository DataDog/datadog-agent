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

// http2DataFrameHeader builds a 9-byte HTTP/2 DATA frame header for a payload
// of the given length on stream 1 with END_STREAM set, mirroring what the
// go-tls write hook captures right before the JSON body.
func http2DataFrameHeader(payloadLen int) []byte {
	return []byte{
		byte(payloadLen >> 16), byte(payloadLen >> 8), byte(payloadLen), // length (24-bit)
		0x00,                   // type = DATA
		0x01,                   // flags = END_STREAM
		0x00, 0x00, 0x00, 0x01, // stream id = 1
	}
}

// padTo emulates the fixed-size eBPF buffer (NUL padded up to size).
func padTo(b []byte, size int) []byte {
	out := make([]byte, size)
	copy(out, b)
	return out
}

func TestParseLLMBody(t *testing.T) {
	openAIBody := `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Explain eBPF observability in one sentence."}]}`

	tests := []struct {
		name       string
		raw        []byte
		wantModel  string
		wantPrompt string
	}{
		{
			name:       "plain json body",
			raw:        []byte(openAIBody),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "Explain eBPF observability in one sentence.",
		},
		{
			name:       "json behind http2 data frame header",
			raw:        append(http2DataFrameHeader(len(openAIBody)), []byte(openAIBody)...),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "Explain eBPF observability in one sentence.",
		},
		{
			name:       "nul padded fixed buffer",
			raw:        padTo(append(http2DataFrameHeader(len(openAIBody)), []byte(openAIBody)...), llmBodyBufferSize),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "Explain eBPF observability in one sentence.",
		},
		{
			name:       "anthropic body with spacing",
			raw:        []byte(`{ "model" : "claude-opus-4-8", "messages" : [ { "role" : "user", "content" : "hi there" } ] }`),
			wantModel:  "claude-opus-4-8",
			wantPrompt: "hi there",
		},
		{
			name:       "truncated before content still gets model",
			raw:        []byte(`{"model":"gpt-4o-mini","messages":[{"role":"user","cont`),
			wantModel:  "gpt-4o-mini",
			wantPrompt: "",
		},
		{
			name:       "empty buffer",
			raw:        nil,
			wantModel:  "",
			wantPrompt: "",
		},
		{
			name:       "all nul buffer",
			raw:        make([]byte, llmBodyBufferSize),
			wantModel:  "",
			wantPrompt: "",
		},
		{
			name:       "non-llm json",
			raw:        []byte(`{"foo":"bar"}`),
			wantModel:  "",
			wantPrompt: "",
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

func TestParseLLMBodyExtractsFirstMessageAsPrompt(t *testing.T) {
	// With a system message followed by a user message, the first content
	// encountered (the system prompt) is returned by this PoC extractor.
	raw := []byte(`{"model":"gpt-4o-mini","messages":[{"role":"system","content":"be terse"},{"role":"user","content":"hello"}]}`)
	model, prompt := parseLLMBody(raw)
	assert.Equal(t, "gpt-4o-mini", model)
	assert.Equal(t, "be terse", prompt)
}

func TestParseLLMMessages(t *testing.T) {
	// Wire format produced by the openai-go SDK: content before role.
	body := `{"messages":[{"content":"You are a concise assistant.","role":"system"},{"content":"what is eBPF?","role":"user"}],"model":"gpt-4o-mini"}`

	tests := []struct {
		name string
		raw  []byte
		want []llmMessage
	}{
		{
			name: "system + user",
			raw:  []byte(body),
			want: []llmMessage{
				{role: "system", content: "You are a concise assistant."},
				{role: "user", content: "what is eBPF?"},
			},
		},
		{
			name: "behind http2 frame header, nul padded",
			raw:  padTo(append(http2DataFrameHeader(len(body)), []byte(body)...), llmBodyBufferSize),
			want: []llmMessage{
				{role: "system", content: "You are a concise assistant."},
				{role: "user", content: "what is eBPF?"},
			},
		},
		{
			name: "single user message",
			raw:  []byte(`{"messages":[{"content":"hi","role":"user"}],"model":"gpt-4o-mini"}`),
			want: []llmMessage{{role: "user", content: "hi"}},
		},
		{
			name: "role-first field order (raw API)",
			raw:  []byte(`{"model":"gpt-4o-mini","messages":[{"role":"system","content":"be terse"},{"role":"user","content":"what is eBPF?"}]}`),
			want: []llmMessage{
				{role: "system", content: "be terse"},
				{role: "user", content: "what is eBPF?"},
			},
		},
		{
			name: "trailing message truncated at buffer boundary is dropped",
			raw:  []byte(`{"messages":[{"content":"sys","role":"system"},{"content":"partial user`),
			want: []llmMessage{{role: "system", content: "sys"}},
		},
		{
			name: "no messages",
			raw:  []byte(`{"model":"gpt-4o-mini"}`),
			want: nil,
		},
		{
			name: "empty",
			raw:  nil,
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLLMMessages(tt.raw, providerOpenAI))
		})
	}
}

func TestParseAnthropicMessages(t *testing.T) {
	// Wire format produced by the anthropic-sdk-go client: user content is an
	// array of text blocks and the system prompt is a top-level array.
	body := `{"max_tokens":100,"messages":[{"content":[{"text":"what is eBPF?","type":"text"}],"role":"user"}],"model":"claude-haiku-4-5","system":[{"text":"You are a concise assistant.","type":"text"}]}`

	tests := []struct {
		name string
		raw  []byte
		want []llmMessage
	}{
		{
			name: "system (top-level) + user (content array)",
			raw:  []byte(body),
			want: []llmMessage{
				{role: "system", content: "You are a concise assistant."},
				{role: "user", content: "what is eBPF?"},
			},
		},
		{
			name: "behind http2 frame header, nul padded",
			raw:  padTo(append(http2DataFrameHeader(len(body)), []byte(body)...), llmBodyBufferSize),
			want: []llmMessage{
				{role: "system", content: "You are a concise assistant."},
				{role: "user", content: "what is eBPF?"},
			},
		},
		{
			name: "no system prompt",
			raw:  []byte(`{"messages":[{"content":[{"text":"hi","type":"text"}],"role":"user"}],"model":"claude-haiku-4-5"}`),
			want: []llmMessage{{role: "user", content: "hi"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseLLMMessages(tt.raw, providerAnthropic))
		})
	}
}

func TestDetectProvider(t *testing.T) {
	assert.Equal(t, providerAnthropic, detectProvider("claude-haiku-4-5"))
	assert.Equal(t, providerAnthropic, detectProvider("claude-3-5-sonnet-20241022"))
	assert.Equal(t, providerOpenAI, detectProvider("gpt-4o-mini"))
	assert.Equal(t, providerOpenAI, detectProvider("o1-preview"))
	assert.Equal(t, providerOpenAI, detectProvider("")) // unknown -> default
}

func TestParseResponseText(t *testing.T) {
	openaiResp := []byte(`{"choices":[{"message":{"role":"assistant","content":"OpenAI answer."}}]}`)
	anthropicResp := []byte(`{"content":[{"type":"text","text":"Anthropic answer."}],"usage":{"input_tokens":9,"output_tokens":4}}`)
	assert.Equal(t, "OpenAI answer.", parseResponseText(openaiResp, providerOpenAI))
	assert.Equal(t, "Anthropic answer.", parseResponseText(anthropicResp, providerAnthropic))
}

func TestParseToolCalls(t *testing.T) {
	// OpenAI: tool_calls[].function.arguments is a JSON-encoded string.
	openaiResp := []byte(`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[` +
		`{"id":"call_abc","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},` +
		`"finish_reason":"tool_calls"}]}`)
	// Anthropic: content[] tool_use block with an input object.
	anthropicResp := []byte(`{"content":[{"type":"tool_use","id":"toolu_01","name":"get_weather","input":{"city":"Paris"}}],` +
		`"stop_reason":"tool_use","usage":{"input_tokens":40,"output_tokens":18}}`)
	// Two OpenAI tool calls in one response.
	openaiMulti := []byte(`"tool_calls":[` +
		`{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}},` +
		`{"id":"call_2","type":"function","function":{"name":"get_time","arguments":"{\"tz\":\"UTC\"}"}}]`)

	tests := []struct {
		name     string
		raw      []byte
		provider string
		want     []llmToolCall
	}{
		{
			name:     "openai single tool call (args unescaped)",
			raw:      openaiResp,
			provider: providerOpenAI,
			want:     []llmToolCall{{id: "call_abc", name: "get_weather", arguments: `{"city":"Paris"}`}},
		},
		{
			name:     "openai two tool calls",
			raw:      openaiMulti,
			provider: providerOpenAI,
			want: []llmToolCall{
				{id: "call_1", name: "get_weather", arguments: `{"city":"Paris"}`},
				{id: "call_2", name: "get_time", arguments: `{"tz":"UTC"}`},
			},
		},
		{
			name:     "anthropic tool_use (input object)",
			raw:      anthropicResp,
			provider: providerAnthropic,
			want:     []llmToolCall{{id: "toolu_01", name: "get_weather", arguments: `{"city":"Paris"}`}},
		},
		{
			name:     "plain text answer -> no tool calls",
			raw:      []byte(`{"choices":[{"message":{"content":"hi"}}]}`),
			provider: providerOpenAI,
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, parseToolCalls(tt.raw, tt.provider))
		})
	}
}

func TestParseToolResults(t *testing.T) {
	// OpenAI follow-up request history: a {"role":"tool",...} message.
	openai := []byte(`{"messages":[{"role":"user","content":"weather?"},` +
		`{"role":"assistant","tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},` +
		`{"role":"tool","tool_call_id":"call_1","content":"18C sunny"}]}`)
	anthropic := []byte(`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"18C sunny"}]}]}`)

	got := parseToolResults(openai, providerOpenAI)
	assert.Equal(t, []llmToolResult{{id: "call_1", content: "18C sunny"}}, got)

	gotA := parseToolResults(anthropic, providerAnthropic)
	assert.Equal(t, []llmToolResult{{id: "toolu_1", content: "18C sunny"}}, gotA)

	assert.Nil(t, parseToolResults([]byte(`{"messages":[{"role":"user","content":"hi"}]}`), providerOpenAI))
}

func TestParseLLMUsage(t *testing.T) {
	tests := []struct {
		name                string
		raw                 []byte
		provider            string
		wantI, wantO, wantT int64
	}{
		{
			name:     "openai response tail with usage",
			raw:      []byte(`"finish_reason":"stop"}],"usage":{"prompt_tokens":17,"completion_tokens":55,"total_tokens":72},"system_fingerprint":"fp_abc"}`),
			provider: providerOpenAI,
			wantI:    17, wantO: 55, wantT: 72,
		},
		{
			name:     "openai nul padded tail",
			raw:      padTo([]byte(`,"usage":{"prompt_tokens":9,"completion_tokens":24,"total_tokens":33}}`), llmBodyBufferSize),
			provider: providerOpenAI,
			wantI:    9, wantO: 24, wantT: 33,
		},
		{
			name:     "openai spaced usage",
			raw:      []byte(`"usage" : { "prompt_tokens" : 100 , "completion_tokens" : 200 , "total_tokens" : 300 }`),
			provider: providerOpenAI,
			wantI:    100, wantO: 200, wantT: 300,
		},
		{
			name:     "anthropic usage (total derived)",
			raw:      []byte(`"stop_reason":"end_turn","usage":{"input_tokens":33,"cache_read_input_tokens":0,"output_tokens":26}}`),
			provider: providerAnthropic,
			wantI:    33, wantO: 26, wantT: 59,
		},
		{
			name:     "anthropic nul padded",
			raw:      padTo([]byte(`,"usage":{"input_tokens":31,"output_tokens":29}}`), llmBodyBufferSize),
			provider: providerAnthropic,
			wantI:    31, wantO: 29, wantT: 60,
		},
		{
			name:     "no usage present",
			raw:      []byte(`{"id":"x","choices":[{"delta":{}}]}`),
			provider: providerOpenAI,
			wantI:    0, wantO: 0, wantT: 0,
		},
		{
			name:     "empty",
			raw:      nil,
			provider: providerOpenAI,
			wantI:    0, wantO: 0, wantT: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in, out, tot := parseLLMUsage(tt.raw, tt.provider)
			assert.Equal(t, tt.wantI, in, "input_tokens")
			assert.Equal(t, tt.wantO, out, "output_tokens")
			assert.Equal(t, tt.wantT, tot, "total_tokens")
		})
	}
}

func TestIsLLMPath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/v1/chat/completions", true},
		{"/v1/completions", true},
		{"/v1/messages", true},
		{"/v1/embeddings", true},
		{"/v1/audio/speech", true},
		{"/v1/models", false},
		{"/health", false},
		{"/", false},
		{"", false},
	}
	for _, tt := range tests {
		assert.Equalf(t, tt.want, isLLMPath([]byte(tt.path)), "path %q", tt.path)
	}
}
