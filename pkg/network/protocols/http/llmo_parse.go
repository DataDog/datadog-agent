// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Typed JSON parsing of captured LLM bodies, preferred over the tolerant regex
// extractors (the *Regex functions in llmo.go).
//
// The captured window is raw decrypted bytes: an optional HTTP/2 frame header,
// the JSON body, and trailing NUL padding, possibly truncated at the buffer
// size. When a complete JSON object decodes we use encoding/json — which,
// unlike the regexes, correctly handles escaped quotes in content, nested
// objects and commas inside tool arguments, unicode escapes, and any field
// order. When no complete object decodes (a truncated body, or a response
// head/tail fragment), we fall back to the regex extractor, which is tolerant
// of partial input.
//
// Request bodies are captured whole (up to the buffer size), so requests parse
// via JSON. Responses are currently captured as head/tail fragments, so they
// usually fall back to regex until body reassembly lands; the response helpers
// therefore only trust a decode that actually produced the expected top-level
// shape (choices/content/usage), so a fragment that happens to contain a valid
// inner object (e.g. the "usage" object in a tail) falls back rather than
// silently decoding the wrong thing.

// openAIToolCallJSON is a tool call in an OpenAI message (request history or
// response). Arguments is a JSON-encoded string; encoding/json unescapes it to
// the raw JSON object on decode.
type openAIToolCallJSON struct {
	ID       string `json:"id"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// openAIMessageJSON is one chat message; Content may be a string, null, or an
// array of content parts.
type openAIMessageJSON struct {
	Role       string               `json:"role"`
	Content    json.RawMessage      `json:"content"`
	ToolCalls  []openAIToolCallJSON `json:"tool_calls"`
	ToolCallID string               `json:"tool_call_id"`
}

// openAIEnvelopeJSON covers both an OpenAI request (messages) and response
// (choices/usage), so a single decode serves either.
type openAIEnvelopeJSON struct {
	Model    string              `json:"model"`
	Messages []openAIMessageJSON `json:"messages"`
	Choices  []struct {
		Message      openAIMessageJSON `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int64 `json:"prompt_tokens"`
		CompletionTokens int64 `json:"completion_tokens"`
		TotalTokens      int64 `json:"total_tokens"`
	} `json:"usage"`
}

// anthropicBlockJSON is one content block; the fields used depend on Type
// (text | tool_use | tool_result).
type anthropicBlockJSON struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	Content   json.RawMessage `json:"content"`
}

// anthropicMessageJSON is one message; Content is a string or an array of blocks.
type anthropicMessageJSON struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

// anthropicEnvelopeJSON covers both an Anthropic request (system/messages) and
// response (content/stop_reason/usage).
type anthropicEnvelopeJSON struct {
	Model      string                 `json:"model"`
	System     json.RawMessage        `json:"system"`
	Messages   []anthropicMessageJSON `json:"messages"`
	Content    []anthropicBlockJSON   `json:"content"`
	StopReason string                 `json:"stop_reason"`
	Usage      struct {
		InputTokens  int64 `json:"input_tokens"`
		OutputTokens int64 `json:"output_tokens"`
	} `json:"usage"`
}

// decodeLLMEnvelope decodes the JSON object in a captured window into T. It
// starts at the first '{' (skipping any HTTP/2 frame header) and decodes one
// value, ignoring trailing NUL padding. It deliberately does NOT retry at later
// '{' positions: on a truncated body that would match an inner object (e.g. a
// single message) and yield an empty-but-valid envelope, hiding the truncation.
// So a truncated body returns ok=false and the caller falls back to the regex
// extractor, which is tolerant of partial input. (A stray '{' inside a frame
// header — only possible when a payload-length byte happens to be 0x7b — also
// falls back to regex, which ignores the header.)
func decodeLLMEnvelope[T any](raw []byte) (T, bool) {
	start := bytes.IndexByte(raw, '{')
	if start < 0 {
		var zero T
		return zero, false
	}
	var v T
	if json.NewDecoder(bytes.NewReader(raw[start:])).Decode(&v) == nil {
		return v, true
	}
	var zero T
	return zero, false
}

// jsonText returns the plain text of a content value that is either a JSON
// string or an array of content blocks ({"type":"text","text":".."}); it
// returns "" for null or any other shape.
func jsonText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return ""
	}
	switch raw[0] {
	case '"':
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	case '[':
		var blocks []anthropicBlockJSON
		if json.Unmarshal(raw, &blocks) == nil {
			var sb strings.Builder
			for _, b := range blocks {
				sb.WriteString(b.Text)
			}
			return sb.String()
		}
	}
	return ""
}

// decodeBlocks decodes a message content that is an array of blocks; nil if it
// is not an array.
func decodeBlocks(raw json.RawMessage) []anthropicBlockJSON {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var blocks []anthropicBlockJSON
	if json.Unmarshal(raw, &blocks) == nil {
		return blocks
	}
	return nil
}

// parseLLMBody extracts the model and the first message content (the prompt
// fallback) from a captured request body, preferring a typed JSON decode.
func parseLLMBody(raw []byte) (model, prompt string) {
	if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok {
		model = env.Model
		for _, m := range env.Messages {
			if c := jsonText(m.Content); c != "" {
				prompt = c
				break
			}
		}
		return model, prompt
	}
	return parseLLMBodyRegex(raw)
}

// parseLLMMessages extracts every chat message (role + content) from a captured
// request body, in order, preferring a typed JSON decode.
func parseLLMMessages(raw []byte, provider string) []llmMessage {
	if len(raw) == 0 {
		return nil
	}
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok {
			return anthropicMessages(env)
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok {
		return openAIMessages(env)
	}
	return parseLLMMessagesRegex(raw, provider)
}

// openAIMessages returns the string-content messages of an OpenAI request, in
// order (assistant tool-call messages, which carry no text content, are
// omitted — mirroring the regex extractor).
func openAIMessages(env openAIEnvelopeJSON) []llmMessage {
	var msgs []llmMessage
	for _, m := range env.Messages {
		if c := jsonText(m.Content); c != "" {
			msgs = append(msgs, llmMessage{role: m.Role, content: c})
		}
	}
	return msgs
}

// anthropicMessages returns the system prompt (top-level, first) followed by the
// text content of each message, matching OpenAI's system-first ordering.
func anthropicMessages(env anthropicEnvelopeJSON) []llmMessage {
	var msgs []llmMessage
	if s := jsonText(env.System); s != "" {
		msgs = append(msgs, llmMessage{role: "system", content: s})
	}
	for _, m := range env.Messages {
		if c := jsonText(m.Content); c != "" {
			msgs = append(msgs, llmMessage{role: m.Role, content: c})
		}
	}
	return msgs
}

// parseResponseText extracts the assistant's answer from a captured response
// body, preferring a typed JSON decode of a complete response.
func parseResponseText(raw []byte, provider string) string {
	if len(raw) == 0 {
		return ""
	}
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok && len(env.Content) > 0 {
			for _, b := range env.Content {
				if b.Type == "text" && b.Text != "" {
					return b.Text
				}
			}
			return ""
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok && len(env.Choices) > 0 {
		return jsonText(env.Choices[0].Message.Content)
	}
	return parseResponseTextRegex(raw, provider)
}

// parseToolCalls extracts tool calls from either a response (the model's
// requested calls) or a request history (a prior turn's calls), preferring a
// typed JSON decode.
func parseToolCalls(raw []byte, provider string) []llmToolCall {
	if len(raw) == 0 {
		return nil
	}
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok && (len(env.Content) > 0 || len(env.Messages) > 0) {
			return anthropicToolCalls(env)
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok && (len(env.Choices) > 0 || len(env.Messages) > 0) {
		return openAIToolCalls(env)
	}
	return parseToolCallsRegex(raw, provider)
}

func openAIToolCalls(env openAIEnvelopeJSON) []llmToolCall {
	var calls []llmToolCall
	collect := func(m openAIMessageJSON) {
		for _, tc := range m.ToolCalls {
			calls = append(calls, llmToolCall{id: tc.ID, name: tc.Function.Name, arguments: tc.Function.Arguments})
		}
	}
	for _, c := range env.Choices {
		collect(c.Message)
	}
	for _, m := range env.Messages {
		collect(m)
	}
	return calls
}

func anthropicToolCalls(env anthropicEnvelopeJSON) []llmToolCall {
	var calls []llmToolCall
	add := func(b anthropicBlockJSON) {
		if b.Type == "tool_use" {
			calls = append(calls, llmToolCall{id: b.ID, name: b.Name, arguments: string(bytes.TrimSpace(b.Input))})
		}
	}
	for _, b := range env.Content {
		add(b)
	}
	for _, m := range env.Messages {
		for _, b := range decodeBlocks(m.Content) {
			add(b)
		}
	}
	return calls
}

// parseToolResults extracts tool results from a captured request body (a
// follow-up call's history), preferring a typed JSON decode.
func parseToolResults(raw []byte, provider string) []llmToolResult {
	if len(raw) == 0 {
		return nil
	}
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok && len(env.Messages) > 0 {
			var res []llmToolResult
			for _, m := range env.Messages {
				for _, b := range decodeBlocks(m.Content) {
					if b.Type == "tool_result" {
						res = append(res, llmToolResult{id: b.ToolUseID, content: jsonText(b.Content)})
					}
				}
			}
			return res
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok && len(env.Messages) > 0 {
		var res []llmToolResult
		for _, m := range env.Messages {
			if m.Role == "tool" {
				res = append(res, llmToolResult{id: m.ToolCallID, content: jsonText(m.Content)})
			}
		}
		return res
	}
	return parseToolResultsRegex(raw, provider)
}

// parseLLMUsage extracts token usage from a captured response body, preferring
// a typed JSON decode of a complete response (a tail fragment, where the usage
// object is nested, falls back to regex).
func parseLLMUsage(raw []byte, provider string) (inputTokens, outputTokens, totalTokens int64) {
	if len(raw) == 0 {
		return 0, 0, 0
	}
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok && (env.Usage.InputTokens > 0 || env.Usage.OutputTokens > 0) {
			return env.Usage.InputTokens, env.Usage.OutputTokens, env.Usage.InputTokens + env.Usage.OutputTokens
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok &&
		(env.Usage.PromptTokens > 0 || env.Usage.CompletionTokens > 0 || env.Usage.TotalTokens > 0) {
		return env.Usage.PromptTokens, env.Usage.CompletionTokens, env.Usage.TotalTokens
	}
	return parseLLMUsageRegex(raw, provider)
}

// isToolCallGen reports whether a response is a tool-call generation, preferring
// a typed JSON decode of a complete response.
func isToolCallGen(raw []byte, provider string) bool {
	if provider == providerAnthropic {
		if env, ok := decodeLLMEnvelope[anthropicEnvelopeJSON](raw); ok && (env.StopReason != "" || len(env.Content) > 0) {
			return env.StopReason == "tool_use"
		}
	} else if env, ok := decodeLLMEnvelope[openAIEnvelopeJSON](raw); ok && len(env.Choices) > 0 {
		for _, c := range env.Choices {
			if c.FinishReason == "tool_calls" {
				return true
			}
		}
		return false
	}
	return isToolCallGenRegex(raw, provider)
}
