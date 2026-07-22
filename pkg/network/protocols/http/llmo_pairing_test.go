// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/network/types"
)

// llmReqEventBytes marshals a request event the way the eBPF ring buffer would,
// so it can be fed to processLLMRequestEvent.
func llmReqEventBytes(t *testing.T, conn llmConnKey, stream, pid uint32, body string) []byte {
	t.Helper()
	var ev llmReqEvent
	ev.Key = conn
	ev.StreamID = stream
	ev.Pid = pid
	ev.Len = uint32(len(body))
	copy(ev.Data[:], body)
	var buf bytes.Buffer
	require.NoError(t, binary.Write(&buf, binary.LittleEndian, &ev))
	return buf.Bytes()
}

// llmRespEventBytes marshals a response event as the eBPF ring buffer would.
func llmRespEventBytes(t *testing.T, conn llmConnKey, stream, endStream uint32, body string) []byte {
	t.Helper()
	var ev llmRespEvent
	ev.Key = conn
	ev.StreamID = stream
	ev.EndStream = endStream
	ev.Len = uint32(len(body))
	copy(ev.Data[:], body)
	var buf bytes.Buffer
	require.NoError(t, binary.Write(&buf, binary.LittleEndian, &ev))
	return buf.Bytes()
}

func newLLMTestStatKeeper(emit func(llmSpanInfo)) *StatKeeper {
	h := &StatKeeper{
		llmReqByStream: make(map[llmStreamKey]llmReqParsed),
		llmRespReasm:   make(map[llmStreamKey]*llmRespReasm),
		llmGenUsage:    make(map[llmConnKey]llmUsage),
	}
	h.llmEmit = func(_ string, _ Method, _ uint16, _ types.ConnectionKey, _ float64, info llmSpanInfo) {
		emit(info)
	}
	return h
}

// TestLLMResponsePairsByStream is the regression test for the "answers are all
// mixed" bug: two conversations share one connection, and the responses arrive
// in the opposite order from the requests. Keying by (conn, stream) must pair
// each response with its own request regardless of arrival order — the previous
// latest-wins-per-connection scheme paired the Berlin question with Paris's
// answer.
func TestLLMResponsePairsByStream(t *testing.T) {
	conn := llmConnKey{SrcIPLow: 1, DstIPLow: 2, SrcPort: 1234, DstPort: 443}
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })

	const berlinReq = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"weather in Berlin?"}]}`
	const parisReq = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"weather in Paris?"}]}`
	const berlinResp = `{"choices":[{"message":{"role":"assistant","content":"It is sunny in Berlin."},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`
	const parisResp = `{"choices":[{"message":{"role":"assistant","content":"It is rainy in Paris."},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":4,"total_tokens":9}}`

	// Berlin on stream 1, Paris on stream 3 — both on the same connection.
	h.processLLMRequestEvent(llmReqEventBytes(t, conn, 1, 4242, berlinReq))
	h.processLLMRequestEvent(llmReqEventBytes(t, conn, 3, 4242, parisReq))

	// Responses arrive in the OPPOSITE order (Paris first). Order must not matter.
	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 3, 0, parisResp))
	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 1, 0, berlinResp))

	require.Len(t, emitted, 2)
	byPrompt := map[string]llmSpanInfo{}
	for _, info := range emitted {
		byPrompt[info.prompt] = info
	}
	assert.Equal(t, "It is sunny in Berlin.", byPrompt["weather in Berlin?"].response,
		"Berlin question must carry Berlin's answer, not another stream's")
	assert.Equal(t, "It is rainy in Paris.", byPrompt["weather in Paris?"].response,
		"Paris question must carry Paris's answer, not another stream's")
	assert.Equal(t, int64(9), byPrompt["weather in Berlin?"].totalTokens, "usage pairs by stream too")
}

// TestLLMResponseReassemblesByStream verifies a response split across multiple
// read events is reassembled per stream (not merged with a concurrent stream's
// reads) before it is paired and emitted.
func TestLLMResponseReassemblesByStream(t *testing.T) {
	conn := llmConnKey{SrcIPLow: 7, DstIPLow: 9, SrcPort: 5555, DstPort: 443}
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })

	const req = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"tell me a long story"}]}`
	h.processLLMRequestEvent(llmReqEventBytes(t, conn, 5, 4242, req))

	// The response comes in two reads on stream 5: the head (no usage yet) then
	// the tail (with usage). Only after the tail is it complete and emitted.
	head := `{"choices":[{"message":{"role":"assistant","content":"once upon a time in a faraway land"},"finish_reason":"stop"}],`
	tail := `"usage":{"prompt_tokens":3,"completion_tokens":20,"total_tokens":23}}`
	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 5, 0, head))
	assert.Empty(t, emitted, "incomplete response (no usage) must not emit yet")

	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 5, 0, tail))
	require.Len(t, emitted, 1)
	assert.Equal(t, "once upon a time in a faraway land", emitted[0].response)
	assert.Equal(t, int64(23), emitted[0].totalTokens)
}

// TestLLMResponseEndStreamFinalizes verifies END_STREAM finalizes a response
// that carries no token usage (an error or a streamed body), so it can't wedge
// the reassembly buffer open — the early-drop defense.
func TestLLMResponseEndStreamFinalizes(t *testing.T) {
	conn := llmConnKey{SrcIPLow: 11, DstIPLow: 13, SrcPort: 6666, DstPort: 443}
	var emitted []llmSpanInfo
	h := newLLMTestStatKeeper(func(info llmSpanInfo) { emitted = append(emitted, info) })

	const req = `{"model":"gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`
	h.processLLMRequestEvent(llmReqEventBytes(t, conn, 7, 4242, req))

	// A response with no usage object: without END_STREAM it would never finalize.
	noUsage := `{"choices":[{"message":{"role":"assistant","content":"partial"}}]}`
	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 7, 0, noUsage))
	assert.Empty(t, emitted, "no usage and no END_STREAM: still accumulating")

	// A terminating read (END_STREAM) finalizes it even without usage.
	h.processLLMResponseEvent(llmRespEventBytes(t, conn, 7, 1, ""))
	require.Len(t, emitted, 1)
	assert.Equal(t, "partial", emitted[0].response)
	_, stillReassembling := h.llmRespReasm[llmStreamKey{conn: conn, stream: 7}]
	assert.False(t, stillReassembling, "reassembly buffer must be dropped after finalize")
}
