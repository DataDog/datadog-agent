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

// makeRespSample encodes an llmRespEvent (connection key + a read window, on
// stream 0) into the raw ring-buffer sample bytes that processLLMResponseEvent
// decodes. Used to feed a single response's reads in chunks.
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

// storeReqOnStream stores a minimal parsed request so the response consumer has
// something to pair its reassembled response with (and therefore emits).
func storeReqOnStream(h *StatKeeper, key llmConnKey, stream uint32, prompt string) {
	h.storeReq(llmStreamKey{conn: key, stream: stream}, llmReqParsed{
		model:    "gpt-4o-mini",
		provider: providerOpenAI,
		prompt:   prompt,
	})
}

// dataFrame wraps payload in an HTTP/2 DATA frame header (type 0x0) for stream.
func dataFrame(stream uint32, payload []byte) []byte {
	n := len(payload)
	hdr := []byte{
		byte(n >> 16), byte(n >> 8), byte(n), 0x00, 0x00,
		byte(stream >> 24), byte(stream >> 16), byte(stream >> 8), byte(stream),
	}
	return append(hdr, payload...)
}

// headersFrame builds a minimal HTTP/2 HEADERS frame (type 0x1) for stream; its
// payload content is irrelevant since de-framing skips non-DATA frames.
func headersFrame(stream uint32) []byte {
	payload := []byte{0x88} // one indexed HPACK byte, content ignored
	n := len(payload)
	return append([]byte{
		byte(n >> 16), byte(n >> 8), byte(n), 0x01, 0x04,
		byte(stream >> 24), byte(stream >> 16), byte(stream >> 8), byte(stream),
	}, payload...)
}

// TestResponseReassemblyLargeAnswer: a large answer that arrives across many
// reads is reassembled in full (not truncated to the first read) before it is
// paired and emitted.
func TestResponseReassemblyLargeAnswer(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	key := llmConnKey{SrcPort: 1111, DstPort: 443}
	storeReqOnStream(h, key, 0, "big question")

	answer := strings.Repeat("eBPF runs sandboxed programs in the kernel safely. ", 220) + " END_ANSWER_MARKER_XYZ"
	framed := openAIResp(answer, "stop")
	require.Greater(t, len(framed), 9000, "answer should be large (multi-read)")

	// Feed the response as ~1.5 KB reads.
	for _, ch := range splitBytes(framed, 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}

	require.Len(t, emitted, 1, "one complete response emits once")
	assert.Equal(t, answer, emitted[0].response, "the full answer must be reassembled, including the trailing marker")
	assert.True(t, strings.HasSuffix(emitted[0].response, "END_ANSWER_MARKER_XYZ"), "trailing marker present")
}

// TestResponseReassemblyIncomplete: before the usage (end of the response) is
// seen, nothing is emitted — the consumer keeps accumulating.
func TestResponseReassemblyIncomplete(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	key := llmConnKey{SrcPort: 2222, DstPort: 443}
	storeReqOnStream(h, key, 0, "q")

	answer := strings.Repeat("partial ", 500)
	framed := openAIResp(answer, "stop")

	// Feed all but the last chunk (which carries the usage / end of JSON).
	chunks := splitBytes(framed, 1500)
	for _, ch := range chunks[:len(chunks)-1] {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	assert.Empty(t, emitted, "must not emit until the response is complete (usage seen)")

	// Final chunk completes it.
	h.processLLMResponseEvent(makeRespSample(key, chunks[len(chunks)-1]))
	require.Len(t, emitted, 1)
	assert.Equal(t, answer, emitted[0].response)
}

// TestResponseReassemblyResetsBetweenResponses: two responses on one connection
// must not be concatenated — the second starts a fresh buffer after the first
// completes.
func TestResponseReassemblyResetsBetweenResponses(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	key := llmConnKey{SrcPort: 3333, DstPort: 443}

	storeReqOnStream(h, key, 0, "q1")
	a1 := strings.Repeat("first answer sentence. ", 120) + " END1"
	for _, ch := range splitBytes(openAIResp(a1, "stop"), 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	require.Len(t, emitted, 1)
	assert.Equal(t, a1, emitted[0].response)

	storeReqOnStream(h, key, 0, "q2")
	a2 := strings.Repeat("second different answer. ", 120) + " END2"
	for _, ch := range splitBytes(openAIResp(a2, "stop"), 1500) {
		h.processLLMResponseEvent(makeRespSample(key, ch))
	}
	require.Len(t, emitted, 2)
	assert.Equal(t, a2, emitted[1].response, "second response must fully replace the first, not append to it")
}

// TestResponseReassemblyToolCallGen: a tool-call generation (finish_reason
// tool_calls) reassembled across reads caches its token usage for the workflow
// and suppresses its own flat span.
func TestResponseReassemblyToolCallGen(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	key := llmConnKey{SrcPort: 4444, DstPort: 443}
	storeReqOnStream(h, key, 0, "weather in Paris?")

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
	require.Len(t, emitted, 1)
	assert.True(t, emitted[0].suppressFlat, "a tool-call generation suppresses its flat span")
}

// TestResponseReassemblyMultiDataFrame reproduces the large-answer truncation
// bug: the body is split across MANY HTTP/2 DATA frames (after a HEADERS frame)
// and delivered across MULTIPLE reads — the first read frame-aligned (stream 1),
// the continuation reads mid-frame (no valid stream id). Per-connection reassembly
// + de-framing must recover the full JSON without truncation.
func TestResponseReassemblyMultiDataFrame(t *testing.T) {
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })
	conn := llmConnKey{SrcPort: 5252, DstPort: 443}
	storeReqOnStream(h, conn, 1, "tell me a long story")

	answer := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 60) + "END_MARKER"
	body := `{"choices":[{"message":{"role":"assistant","content":"` + answer + `"}}],` +
		`"usage":{"prompt_tokens":3,"completion_tokens":90,"total_tokens":93}}`

	// A HEADERS frame, then the body chopped into many small DATA frames.
	framed := headersFrame(1)
	for _, part := range splitBytes([]byte(body), 40) {
		framed = append(framed, dataFrame(1, part)...)
	}

	// Deliver across several reads: only the first is frame-aligned (stream 1);
	// continuation reads carry stream id 0 (mid-frame, no usable id).
	for i, rc := range splitBytes(framed, 1000) {
		stream := uint32(0)
		if i == 0 {
			stream = 1
		}
		h.processLLMResponseEvent(llmRespEventBytes(t, conn, stream, 0, string(rc)))
	}

	require.Len(t, emitted, 1, "multi-frame response should emit exactly once, when complete")
	assert.Equal(t, answer, emitted[0].response, "the full answer must survive multi-DATA-frame de-framing")
	assert.True(t, strings.HasSuffix(emitted[0].response, "END_MARKER"), "not truncated")
	assert.Equal(t, int64(93), emitted[0].totalTokens)
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
