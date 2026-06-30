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
