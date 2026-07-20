// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeRespSample encodes an llmRespEvent (connection key + a read window) into
// the raw ring-buffer sample bytes that processLLMResponseEvent decodes.
func makeRespSample(key llmConnKey, data []byte) []byte {
	var ev llmRespEvent
	ev.Key = key
	ev.Len = uint32(len(data))
	copy(ev.Data[:], data)
	var buf bytes.Buffer
	if err := binary.Write(&buf, binary.LittleEndian, &ev); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

// splitBytes chops b into size-byte chunks, emulating a response arriving over
// several reads.
func splitBytes(b []byte, size int) [][]byte {
	var out [][]byte
	for len(b) > 0 {
		n := size
		if n > len(b) {
			n = len(b)
		}
		out = append(out, b[:n])
		b = b[n:]
	}
	return out
}

// openAIResp builds a complete OpenAI chat response (one HTTP/2 DATA frame) with
// the given assistant answer and a finish reason.
func openAIResp(answer, finish string) []byte {
	body := `{"id":"chatcmpl-x","object":"chat.completion",` +
		`"choices":[{"message":{"role":"assistant","content":"` + answer + `"},"finish_reason":"` + finish + `"}],` +
		`"usage":{"prompt_tokens":12,"completion_tokens":1500,"total_tokens":1512}}`
	return append(http2DataFrameHeader(len(body)), []byte(body)...)
}

func newLLMTestStatKeeper() *StatKeeper {
	return &StatKeeper{
		llmRespContent: make(map[llmConnKey]string),
		llmGenUsage:    make(map[llmConnKey]llmUsage),
	}
}

// TestResponseReassemblyLargeAnswer: a large answer that arrives across many
// reads is reassembled and cached in full (not truncated to the first read).
func TestResponseReassemblyLargeAnswer(t *testing.T) {
	h := newLLMTestStatKeeper()
	key := llmConnKey{SrcPort: 1111, DstPort: 443}

	answer := strings.Repeat("eBPF runs sandboxed programs in the kernel safely. ", 220) + " END_ANSWER_MARKER_XYZ"
	framed := openAIResp(answer, "stop")
	require.Greater(t, len(framed), 9000, "answer should be large (multi-read)")

	// Feed the response as ~1.5 KB reads.
	for _, ch := range splitBytes(framed, 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}

	got, ok := h.lookupRespContent(key)
	assert.True(t, ok, "answer should be cached")
	assert.Equal(t, answer, got, "the full answer must be reassembled, including the trailing marker")
	assert.True(t, strings.HasSuffix(got, "END_ANSWER_MARKER_XYZ"), "trailing marker present")
}

// TestResponseReassemblyIncomplete: before the usage (end of the response) is
// seen, nothing is cached — the consumer keeps accumulating.
func TestResponseReassemblyIncomplete(t *testing.T) {
	h := newLLMTestStatKeeper()
	key := llmConnKey{SrcPort: 2222, DstPort: 443}

	answer := strings.Repeat("partial ", 500)
	framed := openAIResp(answer, "stop")

	// Feed all but the last chunk (which carries the usage / end of JSON).
	chunks := splitBytes(framed, 1500)
	for _, ch := range chunks[:len(chunks)-1] {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	_, ok := h.lookupRespContent(key)
	assert.False(t, ok, "must not cache until the response is complete (usage seen)")

	// Final chunk completes it.
	h.processLLMResponseEvent(makeRespSample(key, chunks[len(chunks)-1]))
	got, ok := h.lookupRespContent(key)
	assert.True(t, ok)
	assert.Equal(t, answer, got)
}

// TestResponseReassemblyResetsBetweenResponses: two responses on one connection
// must not be concatenated — the second resets after the first completes.
func TestResponseReassemblyResetsBetweenResponses(t *testing.T) {
	h := newLLMTestStatKeeper()
	key := llmConnKey{SrcPort: 3333, DstPort: 443}

	a1 := strings.Repeat("first answer sentence. ", 120) + " END1"
	for _, ch := range splitBytes(openAIResp(a1, "stop"), 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	got1, ok := h.lookupRespContent(key)
	assert.True(t, ok)
	assert.Equal(t, a1, got1)

	a2 := strings.Repeat("second different answer. ", 120) + " END2"
	for _, ch := range splitBytes(openAIResp(a2, "stop"), 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	got2, ok := h.lookupRespContent(key)
	assert.True(t, ok)
	assert.Equal(t, a2, got2, "second response must fully replace the first, not append to it")
}

// TestResponseReassemblyToolCallGen: a tool-call generation (finish_reason
// tool_calls) reassembled across reads caches its token usage for the workflow.
func TestResponseReassemblyToolCallGen(t *testing.T) {
	h := newLLMTestStatKeeper()
	key := llmConnKey{SrcPort: 4444, DstPort: 443}

	body := `{"id":"chatcmpl-t","object":"chat.completion",` +
		`"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[` +
		`{"id":"call_1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Paris\"}"}}]},` +
		`"finish_reason":"tool_calls"}],` +
		`"usage":{"prompt_tokens":40,"completion_tokens":18,"total_tokens":58}}`
	framed := append(http2DataFrameHeader(len(body)), []byte(body)...)
	for _, ch := range splitBytes(framed, 64) { // tiny reads to stress reassembly
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	u, ok := h.lookupGenUsage(key)
	assert.True(t, ok, "tool-call generation usage should be cached")
	assert.Equal(t, int64(58), u.total)
}

// TestParseLargeRequestPrompt: a large user prompt in a request body (behind a
// frame header, NUL-padded to the buffer) is parsed in full.
func TestParseLargeRequestPrompt(t *testing.T) {
	prompt := strings.Repeat("Explain this topic in detail. ", 400) + " END_PROMPT_MARKER_ABC"
	req := `{"model":"gpt-4o-mini","messages":[` +
		`{"role":"system","content":"be helpful"},` +
		`{"role":"user","content":"` + prompt + `"}]}`
	require.Less(t, len(req), llmReqBodyBufferSize, "prompt fits one request window")
	raw := padTo(append(http2DataFrameHeader(len(req)), []byte(req)...), llmReqBodyBufferSize)

	msgs := parseLLMMessages(raw, providerOpenAI)
	require.Len(t, msgs, 2)
	assert.Equal(t, "user", msgs[1].role)
	assert.Equal(t, prompt, msgs[1].content, "the full large prompt must be parsed, marker included")
	assert.True(t, strings.HasSuffix(msgs[1].content, "END_PROMPT_MARKER_ABC"))

	model, _ := parseLLMBody(raw)
	assert.Equal(t, "gpt-4o-mini", model)
}

// TestParseResponseTextLarge: parsing a large answer directly returns it whole.
func TestParseResponseTextLarge(t *testing.T) {
	answer := strings.Repeat("token ", 2000) + "TAIL_MARKER_QRS"
	resp := `{"choices":[{"message":{"content":"` + answer + `"}}],"usage":{"total_tokens":5}}`
	got := parseResponseText([]byte(resp), providerOpenAI)
	assert.Equal(t, answer, got)
	assert.True(t, strings.HasSuffix(got, "TAIL_MARKER_QRS"))
}
