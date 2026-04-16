// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const testMaxContentSize = 256 * 1024

func makeMsg(content string) *message.Message {
	msg := message.NewMessage([]byte(content), nil, "", 0)
	msg.RawDataLen = len(content)
	return msg
}

// feedLines splits a multi-line input into individual messages, feeds them to
// the aggregator one at a time, and collects all output messages.
func feedLines(agg StackTraceAggregator, input string) []*message.Message {
	var out []*message.Message
	lines := strings.Split(input, "\n")
	// strings.Split produces a trailing empty string for inputs ending in "\n"
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		out = append(out, agg.Process(makeMsg(line))...)
	}
	out = append(out, agg.Flush()...)
	return out
}

// assertCombined checks that exactly one message was produced and that it has IsMultiLine set.
func assertCombined(t *testing.T, msgs []*message.Message) {
	t.Helper()
	require.Len(t, msgs, 1, "expected exactly 1 combined message")
	assert.True(t, msgs[0].ParsingExtra.IsMultiLine, "expected IsMultiLine to be true on combined message")
}

// assertAbandoned checks that >1 messages were produced and none has IsMultiLine set.
func assertAbandoned(t *testing.T, msgs []*message.Message, minCount int) {
	t.Helper()
	require.GreaterOrEqual(t, len(msgs), minCount, "expected at least %d individual messages", minCount)
	for i, m := range msgs {
		assert.False(t, m.ParsingExtra.IsMultiLine, "message %d should not have IsMultiLine", i)
	}
}

// ---------------------------------------------------------------------------
// Ported positive tests from parser_test.go
// ---------------------------------------------------------------------------

func TestGoStackTrace_Pattern1_PlainPanic(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: something went wrong\n\ngoroutine 1 [running]:\nmain.plainPanic(...)\n\t/path/main.go:81\nmain.main()\n\t/path/main.go:46 +0x5b4\n")
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern2_FatalErrorConcurrentMap(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "fatal error: concurrent map writes\n\ngoroutine 47 [running]:\ninternal/runtime/maps.fatal({0x1048bc2e5?, 0x0?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:1181 +0x20\nmain.concurrentMapFatal.func1(0x1c)\n\t/path/main.go:93 +0x60\ncreated by main.concurrentMapFatal in goroutine 1\n\t/path/main.go:90 +0x48\n")
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern3_PanicPlusSignal(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: runtime error: invalid memory address or nil pointer dereference\n[signal SIGSEGV: segmentation violation code=0x2 addr=0x0 pc=0x10436ffd0]\n\ngoroutine 1 [running]:\nmain.nilDerefSignalPanic()\n\t/path/main.go:103 +0x20\nmain.main()\n\t/path/main.go:50 +0x3b4\n")
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern4_SIGSEGVRegisterDump(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "SIGSEGV: segmentation violation\nPC=0x192bf82f4 m=0 sigcode=2 addr=0x0\n\ngoroutine 0 gp=0x1021a3080 m=0 mp=0x1021a3880 [idle]:\nruntime.usleep(0x3)\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/sys_darwin.go:274 +0x1c fp=0x16ddee040 sp=0x16ddee020 pc=0x10206f8ec\n\ngoroutine 1 gp=0x7df94bf261e0 m=nil [sleep]:\nruntime.gopark(0x10404bfa09498?, 0x1665?, 0xb?, 0x0?, 0x1?)\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/proc.go:462 +0xbc fp=0x7df94bfabd50 sp=0x7df94bfabd30 pc=0x102082b2c\n\nr0\t0x332a97571dd8\nr1\t0x10291c6a8\nlr\t0x1028f0c2c\nsp\t0x16d586380\npc\t0x10291c6a8\nfault\t0x10291c6a8\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern5_SIGTRAPCgo(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "SIGTRAP: trace trap\nPC=0x10291c6a8 m=0 sigcode=0\nsignal arrived during cgo execution\n\ngoroutine 1 gp=0x332a974ec1e0 m=0 mp=0x102a0b880 [syscall]:\nruntime.cgocall(0x10291c6a8, 0x332a97571dd8)\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/cgocall.go:167 +0x44 fp=0x332a97571da0 sp=0x332a97571d60 pc=0x1028e8dc4\n\nr0\t0x332a97571dd8\nr1\t0x10291c6a8\nsp\t0x16d586380\npc\t0x10291c6a8\nfault\t0x10291c6a8\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern6_PanicOnSystemStack(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: panic on system stack\nfatal error: panic on system stack\n\nruntime stack:\nruntime.throw({0x104ebbfb2?, 0x104f980b0?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:1229 +0x38 fp=0x16afea2c0 sp=0x16afea290 pc=0x104e86a18\nruntime.systemstack(0x0)\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/asm_arm64.s:399 +0x68 fp=0x16afea3a0 sp=0x16afea390 pc=0x104e8acb8\n\ngoroutine 1 gp=0x14d4aaf241e0 m=0 mp=0x104fa7880 [running]:\nruntime.systemstack_switch()\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/asm_arm64.s:347 +0x8 fp=0x14d4aafa9dc0 sp=0x14d4aafa9db0 pc=0x104e8ac38\nmain.main()\n\t/path/main.go:56 +0x3e0 fp=0x14d4aafa9f30 sp=0x14d4aafa9de0 pc=0x104eb7b60\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern7_PanicHoldingLocks(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: panic holding locks\n\ngoroutine 1 [running]:\nmain.panicHoldingLocks(...)\n\t/path/main.go:164\nmain.main()\n\t/path/main.go:58 +0x574\n")
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern8_MultiLinePanicValue(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: first line of error\n\tsecond line of error\n\tthird line\n\ngoroutine 1 [running]:\nmain.multiLinePanicValue(...)\n\t/path/main.go:170\nmain.main()\n\t/path/main.go:60 +0x528\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern9_NestedPanicChain(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: outer panic [recovered]\n\tpanic: inner panic\n\ngoroutine 1 [running]:\nmain.nestedPanicChain.func1()\n\t/path/main.go:178 +0x30\npanic({0x104fc70a0?, 0x104fe4090?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:860 +0x12c\nmain.nestedPanicChain()\n\t/path/main.go:180 +0x48\nmain.main()\n\t/path/main.go:62 +0x40c\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern11_StackOverflow(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "runtime: goroutine stack exceeds 1000000000-byte limit\nruntime: sp=0x54fa64e03a0 stack=[0x54fa64e0000, 0x54fc64e0000]\nfatal error: stack overflow\n\nruntime stack:\nruntime.throw({0x1049329bc?, 0x1048d40ac?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:1229 +0x38 fp=0x16b572250 sp=0x16b572220 pc=0x1048fea18\nruntime.newstack()\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/stack.go:1178 +0x498 fp=0x16b572380 sp=0x16b572250 pc=0x1048e7718\nruntime.morestack()\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/asm_arm64.s:507 +0x70 fp=0x16b572380 sp=0x16b572380 pc=0x104902dc0\n\ngoroutine 1 gp=0x54f863441e0 m=0 mp=0x104a1f880 [running]:\nmain.stackOverflow()\n\t/path/main.go:191 +0x30 fp=0x54fa64e03a0 sp=0x54fa64e03a0 pc=0x104930190\nmain.stackOverflow()\n\t/path/main.go:192 +0x1c fp=0x54fa64e03b0 sp=0x54fa64e03a0 pc=0x10493017c\n...33554244 frames elided...\nmain.stackOverflow()\n\t/path/main.go:192 +0x1c fp=0x54fc64dfde0 sp=0x54fc64dfdd0 pc=0x10493017c\nmain.main()\n\t/path/main.go:66 +0x444 fp=0x54fc64dff30 sp=0x54fc64dfde0 pc=0x10492fbc4\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern13_OutOfMemory(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "runtime: out of memory: cannot allocate 140737488355328-byte block (3833856 in use)\nfatal error: out of memory\n\ngoroutine 1 gp=0x70edbce0c1e0 m=0 mp=0x10243f880 [running]:\nruntime.throw({0x1023524f0?, 0x1022c44d8?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:1229 +0x38 fp=0x70edbce90cb0 sp=0x70edbce90c80 pc=0x10231ea18\nmain.outOfMemory()\n\t/path/main.go:231 +0x2c fp=0x70edbce90de0 sp=0x70edbce90db0 pc=0x10235047c\nmain.main()\n\t/path/main.go:70 +0x47c fp=0x70edbce90f30 sp=0x70edbce90de0 pc=0x10234fbfc\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_Pattern14_UnexpectedFault(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "unexpected fault address 0x10524c000\nfatal error: fault\n[signal SIGSEGV: segmentation violation code=0x2 addr=0x10524c000 pc=0x104f24518]\n\ngoroutine 1 gp=0x6cb7e7e581e0 m=0 mp=0x105013880 [running]:\nruntime.throw({0x104f2532f?, 0x104f24514?})\n\t/opt/homebrew/Cellar/go/1.26.0/libexec/src/runtime/panic.go:1229 +0x38 fp=0x6cb7e7ed8d20 sp=0x6cb7e7ed8cf0 pc=0x104ef2a18\nmain.unexpectedFault()\n\t/path/main.go:258 +0x78 fp=0x6cb7e7ed8de0 sp=0x6cb7e7ed8d90 pc=0x104f24518\nmain.main()\n\t/path/main.go:72 +0x474 fp=0x6cb7e7ed8f30 sp=0x6cb7e7ed8de0 pc=0x104f23bf4\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_PartialCrash_HeaderPlusGoroutineHeader(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: something went wrong\n\ngoroutine 1 [running]:\n")
	assertCombined(t, msgs)
}

func TestGoStackTrace_NoTrailingNewline(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: bad\n\ngoroutine 1 [running]:\nmain.main()\n\t/path/main.go:1 +0x1")
	assertCombined(t, msgs)
}

// ---------------------------------------------------------------------------
// Ported negative tests from parser_test.go — these should NOT combine
// ---------------------------------------------------------------------------

func TestGoStackTrace_EmptyInput(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "")
	assert.Empty(t, msgs)
}

func TestGoStackTrace_BlankLinesOnly(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "\n\n\n")
	for _, m := range msgs {
		assert.False(t, m.ParsingExtra.IsMultiLine)
	}
}

func TestGoStackTrace_RandomGarbage(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "hello world\nfoo bar\n")
	assertAbandoned(t, msgs, 2)
}

func TestGoStackTrace_LogLineWithTimestamp(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "2024-01-01T00:00:00Z INFO starting server\n")
	assertAbandoned(t, msgs, 1)
}

func TestGoStackTrace_HeaderOnly_NoStacks(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: something went wrong\n")
	assertAbandoned(t, msgs, 1)
}

func TestGoStackTrace_HeaderPlusBlank_NoChunk(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: something went wrong\n\n")
	assertAbandoned(t, msgs, 1)
}

func TestGoStackTrace_HeaderWithGarbageContinuation(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "panic: something went wrong\nTHIS IS NOT A VALID CONTINUATION\n\ngoroutine 1 [running]:\nmain.main()\n\t/path/main.go:46 +0x5b4\n")
	for _, m := range msgs {
		assert.False(t, m.ParsingExtra.IsMultiLine)
	}
}

func TestGoStackTrace_ValidHeaderGarbageAfterFirstChunk(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: something went wrong\n\ngoroutine 1 [running]:\nmain.main()\n\t/path/main.go:46 +0x5b4\n\nTHIS IS GARBAGE\n"
	msgs := feedLines(agg, input)
	// The aggregator should combine the valid part and emit the garbage line separately.
	// The valid part has chunkCount > 0, so it should be combined.
	combined := false
	for _, m := range msgs {
		if m.ParsingExtra.IsMultiLine {
			combined = true
		}
	}
	assert.True(t, combined, "expected the valid crash portion to be combined")
	assert.GreaterOrEqual(t, len(msgs), 2, "expected at least 2 messages (combined + garbage)")
}

func TestGoStackTrace_GarbageInsideGoroutineChunk(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: something went wrong\n\ngoroutine 1 [running]:\nmain.main()\n12345 this is not a valid stack line\n\t/path/main.go:46 +0x5b4\n"
	msgs := feedLines(agg, input)
	// The garbage line rejects the chunk. chunkCount is 1 at this point (goroutine header counted),
	// but the func/file alternation is broken. Depending on the exact state, this may combine
	// what was valid or abandon. The goroutine header was seen so chunkCount=1, meaning
	// the buffer up to rejection is combined, and the garbage line is emitted separately.
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestGoStackTrace_FatalErrorHeaderOnly(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "fatal error: concurrent map writes\n")
	assertAbandoned(t, msgs, 1)
}

func TestGoStackTrace_SignalHeaderOnly(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	msgs := feedLines(agg, "SIGSEGV: segmentation violation\nPC=0x192bf82f4 m=0 sigcode=2 addr=0x0\n")
	assertAbandoned(t, msgs, 1)
}

// ---------------------------------------------------------------------------
// Alternation violation tests (ported from parser_test.go)
// ---------------------------------------------------------------------------

func TestGoStackTrace_TwoConsecutiveFuncLines(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: bad\n\ngoroutine 1 [running]:\nmain.foo()\nmain.bar()\n\t/path/main.go:1 +0x1\n"
	msgs := feedLines(agg, input)
	// main.foo() starts chunk (chunkCount=1), expects file next.
	// main.bar() is func line where file is expected -> rejection.
	// chunkCount=1 so buffer is combined, then main.bar() + rest emitted individually.
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestGoStackTrace_TwoConsecutiveTabLines(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: bad\n\ngoroutine 1 [running]:\nmain.foo()\n\t/path/main.go:1 +0x1\n\t/path/main.go:2 +0x2\n"
	msgs := feedLines(agg, input)
	// After first tab line, expectFile=false. Second tab line is rejected (expects func).
	// chunkCount=1 so buffer is combined.
	assert.GreaterOrEqual(t, len(msgs), 2)
}

func TestGoStackTrace_TabRightAfterGoroutineHeader(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: bad\n\ngoroutine 1 [running]:\n\t/path/main.go:1 +0x1\n"
	msgs := feedLines(agg, input)
	// goroutine header starts chunk (chunkCount=1), expectFile=false.
	// Tab line is rejected because we expect func, not file.
	// chunkCount=1 so the buffer up to rejection is combined.
	assert.GreaterOrEqual(t, len(msgs), 2)
}

// ---------------------------------------------------------------------------
// Abandon tests: false positive headers with chunkCount == 0
// ---------------------------------------------------------------------------

func TestGoStackTrace_Abandon_FalsePositiveHeader(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: just a log message that starts with panic\nsome normal log line\nanother normal log line\n"
	msgs := feedLines(agg, input)
	// "panic:" matches the start marker, but "some normal log line" is not a valid header continuation.
	// chunkCount == 0, so all buffered lines are released individually.
	assertAbandoned(t, msgs, 3)
}

func TestGoStackTrace_Abandon_HeaderPlusSignal_NoGoroutines(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: runtime error: nil pointer dereference\n[signal SIGSEGV: segmentation violation code=0x2 addr=0x0 pc=0x10436ffd0]\n\nsome unrelated line\n"
	msgs := feedLines(agg, input)
	// Header + signal is valid header continuation, then empty line transitions to betweenChunks.
	// "some unrelated line" is not a valid chunk start, so state machine rejects.
	// chunkCount == 0, so all lines are abandoned.
	for _, m := range msgs {
		assert.False(t, m.ParsingExtra.IsMultiLine, "expected all messages to have IsMultiLine=false")
	}
}

func TestGoStackTrace_Abandon_FlushWithChunkCountZero(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	// Feed just a header + empty line, then flush. chunkCount == 0.
	out := agg.Process(makeMsg("panic: something"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Flush()
	assertAbandoned(t, out, 2)
}

// ---------------------------------------------------------------------------
// Combine vs abandon boundary tests
// ---------------------------------------------------------------------------

func TestGoStackTrace_CombineAfterOneChunk(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	// header + empty + goroutine header + func + file + empty + rejection
	// chunkCount=1 at rejection point -> should combine
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.main()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:1 +0x1"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("THIS IS NOT A CHUNK START"))
	// The buffer should be resolved: chunkCount=1 -> combine.
	// The rejected line should be emitted separately.
	require.Len(t, out, 2)
	assert.True(t, out[0].ParsingExtra.IsMultiLine, "combined crash message should have IsMultiLine")
	assert.False(t, out[1].ParsingExtra.IsMultiLine, "rejected line should not have IsMultiLine")
}

func TestGoStackTrace_AbandonBeforeAnyChunk(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	// header -> rejection before any chunk starts
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("NOT VALID CONTINUATION"))
	// chunkCount==0, buffer abandoned: panic line + rejected line
	require.Len(t, out, 2)
	for _, m := range out {
		assert.False(t, m.ParsingExtra.IsMultiLine)
	}
}

// ---------------------------------------------------------------------------
// Uncommitted rollback tests
// ---------------------------------------------------------------------------

func TestGoStackTrace_Rollback_UnmatchedFuncName(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.foo()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:42 +0x1"))
	assert.Empty(t, out)
	// "exit status 2" is accepted as a function name (expectFile=true, uncommitted=1)
	out = agg.Process(makeMsg("exit status 2"))
	assert.Empty(t, out)
	// Next line is not a tab -> rejection -> resolve with rollback
	out = agg.Process(makeMsg("2024-01-01 INFO next log"))
	require.Len(t, out, 3, "combined trace + rolled-back func + rejected line")
	assert.True(t, out[0].ParsingExtra.IsMultiLine, "trace should be combined")
	assert.NotContains(t, string(out[0].GetContent()), "exit status 2",
		"combined message must not include the tentative function name")
	assert.Equal(t, "exit status 2", string(out[1].GetContent()),
		"rolled-back line should be emitted individually")
	assert.Equal(t, "2024-01-01 INFO next log", string(out[2].GetContent()),
		"rejected line should be emitted individually")
}

func TestGoStackTrace_Rollback_FuncThenBlankLine(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.foo()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:42 +0x1"))
	assert.Empty(t, out)
	// Unmatched function name
	out = agg.Process(makeMsg("exit status 2"))
	assert.Empty(t, out)
	// Empty line after tentative func -> uncommitted=2
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	// Now a non-chunk-start line triggers rejection
	out = agg.Process(makeMsg("some normal log"))
	require.Len(t, out, 4, "combined + func + blank + rejected")
	assert.True(t, out[0].ParsingExtra.IsMultiLine)
	assert.NotContains(t, string(out[0].GetContent()), "exit status 2")
	assert.Equal(t, "exit status 2", string(out[1].GetContent()))
	assert.Equal(t, "", string(out[2].GetContent()))
	assert.Equal(t, "some normal log", string(out[3].GetContent()))
}

func TestGoStackTrace_Rollback_CompletePairNotRolledBack(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.foo()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:42 +0x1"))
	assert.Empty(t, out)
	// Flush with a complete func/file pair -> uncommitted=0, no rollback
	out = agg.Flush()
	require.Len(t, out, 1)
	assert.True(t, out[0].ParsingExtra.IsMultiLine)
	assert.Contains(t, string(out[0].GetContent()), "main.foo()")
	assert.Contains(t, string(out[0].GetContent()), "/path/main.go:42")
}

func TestGoStackTrace_Rollback_ChunkBoundaryResetsUncommitted(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.foo()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:42 +0x1"))
	assert.Empty(t, out)
	// Tentative function name
	out = agg.Process(makeMsg("exit status 2"))
	assert.Empty(t, out)
	// Empty line -> uncommitted=2, transitions to betweenChunks
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	// New chunk starts -> uncommitted resets to 0
	out = agg.Process(makeMsg("goroutine 2 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.bar()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:99 +0x2"))
	assert.Empty(t, out)
	// Flush with 2 complete chunks -> no rollback
	out = agg.Flush()
	require.Len(t, out, 1)
	assert.True(t, out[0].ParsingExtra.IsMultiLine)
	combined := string(out[0].GetContent())
	assert.Contains(t, combined, "goroutine 1")
	assert.Contains(t, combined, "goroutine 2")
	assert.Contains(t, combined, "exit status 2",
		"exit status 2 should be included since the chunk boundary committed it")
}

func TestGoStackTrace_Rollback_FlushWithUncommitted(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.foo()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:42 +0x1"))
	assert.Empty(t, out)
	// Tentative func name, then flush
	out = agg.Process(makeMsg("exit status 2"))
	assert.Empty(t, out)
	out = agg.Flush()
	require.Len(t, out, 2, "combined trace + rolled-back func")
	assert.True(t, out[0].ParsingExtra.IsMultiLine)
	assert.NotContains(t, string(out[0].GetContent()), "exit status 2")
	assert.Equal(t, "exit status 2", string(out[1].GetContent()))
}

// ---------------------------------------------------------------------------
// maxContentSize overflow tests
// ---------------------------------------------------------------------------

func TestGoStackTrace_Overflow_AbandonRegardlessOfChunkCount(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), 100) // very small max
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.main()"))
	assert.Empty(t, out)
	// This should push us over 100 bytes and trigger overflow abandon
	out = agg.Process(makeMsg("\t/path/main.go:1 +0x1 this-is-a-very-long-file-path-to-push-over-the-limit"))
	// All buffered lines + this line should be emitted individually
	require.GreaterOrEqual(t, len(out), 2)
	for _, m := range out {
		assert.False(t, m.ParsingExtra.IsMultiLine, "overflow should not produce IsMultiLine messages")
	}
}

func TestGoStackTrace_Overflow_SmallBuffer(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), 30) // tiny
	out := agg.Process(makeMsg("panic: very long panic message"))
	assert.Empty(t, out)
	// Next line triggers overflow
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	// "goroutine 1..." is not a valid header continuation, so it triggers rejection
	// with chunkCount==0 -> abandoned, NOT overflow
	for _, m := range out {
		assert.False(t, m.ParsingExtra.IsMultiLine)
	}
}

// ---------------------------------------------------------------------------
// New start marker during buffering
// ---------------------------------------------------------------------------

func TestGoStackTrace_NewStartMarkerResolvesOld(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	// First crash
	out := agg.Process(makeMsg("panic: first"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.main()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:1 +0x1"))
	assert.Empty(t, out)
	// Empty line transitions to betweenChunks where a new start marker is rejected
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)

	// New crash starts — rejected by betweenChunks (not a valid chunk start),
	// and since it matches IsStart, the old buffer is resolved and
	// a new trace begins.
	out = agg.Process(makeMsg("panic: second"))
	require.Len(t, out, 1)
	assert.True(t, out[0].ParsingExtra.IsMultiLine, "first crash should be combined")

	// Flush the second (header-only, chunkCount==0)
	out = agg.Flush()
	assertAbandoned(t, out, 1)
}

// ---------------------------------------------------------------------------
// Noop variant tests
// ---------------------------------------------------------------------------

func TestNoopStackTraceAggregator_PassThrough(t *testing.T) {
	noop := NewNoopStackTraceAggregator()
	msg := makeMsg("panic: hello")
	out := noop.Process(msg)
	require.Len(t, out, 1)
	assert.Equal(t, msg, out[0])
	assert.False(t, out[0].ParsingExtra.IsMultiLine)
}

func TestNoopStackTraceAggregator_FlushReturnsNil(t *testing.T) {
	noop := NewNoopStackTraceAggregator()
	assert.Nil(t, noop.Flush())
}

func TestNoopStackTraceAggregator_IsEmptyAlwaysTrue(t *testing.T) {
	noop := NewNoopStackTraceAggregator()
	assert.True(t, noop.IsEmpty())
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestGoStackTrace_ElidedFrames(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "panic: bad\n\ngoroutine 1 [running]:\nmain.foo()\n\t/path/main.go:1 +0x1\n...100 frames elided...\nmain.bar()\n\t/path/main.go:2 +0x2\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_CreatedByLine(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	input := "fatal error: concurrent map writes\n\ngoroutine 47 [running]:\nmain.handler()\n\t/path/main.go:93 +0x60\ncreated by main.main in goroutine 1\n\t/path/main.go:90 +0x48\n"
	msgs := feedLines(agg, input)
	assertCombined(t, msgs)
}

func TestGoStackTrace_IsEmpty(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	assert.True(t, agg.IsEmpty())
	agg.Process(makeMsg("panic: test"))
	assert.False(t, agg.IsEmpty())
	agg.Flush()
	assert.True(t, agg.IsEmpty())
}

func TestGoStackTrace_CombinedContentContainsAllLines(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.main()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:1 +0x1"))
	assert.Empty(t, out)
	out = agg.Flush()
	require.Len(t, out, 1)
	assert.True(t, out[0].ParsingExtra.IsMultiLine)

	content := string(out[0].GetContent())
	assert.Contains(t, content, "panic: bad")
	assert.Contains(t, content, "goroutine 1 [running]:")
	assert.Contains(t, content, "main.main()")
	assert.Contains(t, content, "/path/main.go:1 +0x1")
}

func TestGoStackTrace_RawDataLenAggregated(t *testing.T) {
	agg := NewStackTraceAggregator(NewGoStackTraceParser(), testMaxContentSize)
	m1 := makeMsg("panic: bad")
	m1.RawDataLen = 15
	m2 := makeMsg("")
	m2.RawDataLen = 5
	m3 := makeMsg("goroutine 1 [running]:")
	m3.RawDataLen = 27
	m4 := makeMsg("main.main()")
	m4.RawDataLen = 16
	m5 := makeMsg("\t/path/main.go:1 +0x1")
	m5.RawDataLen = 25

	agg.Process(m1)
	agg.Process(m2)
	agg.Process(m3)
	agg.Process(m4)
	agg.Process(m5)
	out := agg.Flush()
	require.Len(t, out, 1)
	assert.Equal(t, 15+5+27+16+25, out[0].RawDataLen)
}

// ---------------------------------------------------------------------------
// CompositeStackTraceParser tests
// ---------------------------------------------------------------------------

// mockParser is a minimal StackTraceParser for testing the composite.
type mockParser struct {
	startPrefix string
	accepted    int
	resetCalled bool
}

func (m *mockParser) IsStart(content []byte) bool {
	return len(content) >= len(m.startPrefix) && string(content[:len(m.startPrefix)]) == m.startPrefix
}

func (m *mockParser) Reset() {
	m.resetCalled = true
	m.accepted = 0
}

func (m *mockParser) AcceptLine(line []byte) bool {
	if len(line) > 0 && line[0] == '\t' {
		m.accepted++
		return true
	}
	return false
}

func (m *mockParser) ShouldCombine() bool {
	return m.accepted > 0
}

func (m *mockParser) Uncommitted() int {
	return 0
}

func TestCompositeStackTraceParser_DelegatesToMatchingParser(t *testing.T) {
	goMock := &mockParser{startPrefix: "panic:"}
	javaMock := &mockParser{startPrefix: "Exception"}

	comp := NewCompositeStackTraceParser([]StackTraceParser{goMock, javaMock})

	assert.True(t, comp.IsStart([]byte("panic: bad")))
	comp.Reset()
	assert.True(t, goMock.resetCalled, "Go mock should have been reset")
	assert.False(t, javaMock.resetCalled, "Java mock should not have been reset")

	assert.True(t, comp.AcceptLine([]byte("\tstack frame")))
	assert.True(t, comp.ShouldCombine())
}

func TestCompositeStackTraceParser_SecondParserMatches(t *testing.T) {
	goMock := &mockParser{startPrefix: "panic:"}
	javaMock := &mockParser{startPrefix: "Exception"}

	comp := NewCompositeStackTraceParser([]StackTraceParser{goMock, javaMock})

	assert.True(t, comp.IsStart([]byte("Exception in thread")))
	comp.Reset()
	assert.False(t, goMock.resetCalled, "Go mock should not have been reset")
	assert.True(t, javaMock.resetCalled, "Java mock should have been reset")
}

func TestCompositeStackTraceParser_NoMatch(t *testing.T) {
	goMock := &mockParser{startPrefix: "panic:"}
	javaMock := &mockParser{startPrefix: "Exception"}

	comp := NewCompositeStackTraceParser([]StackTraceParser{goMock, javaMock})

	assert.False(t, comp.IsStart([]byte("INFO normal log")))
	assert.False(t, comp.AcceptLine([]byte("anything")))
	assert.False(t, comp.ShouldCombine())
}

func TestCompositeStackTraceParser_PriorityOrder(t *testing.T) {
	first := &mockParser{startPrefix: "panic:"}
	second := &mockParser{startPrefix: "panic:"}

	comp := NewCompositeStackTraceParser([]StackTraceParser{first, second})
	assert.True(t, comp.IsStart([]byte("panic: both match")))
	comp.Reset()
	assert.True(t, first.resetCalled, "First parser should win by priority")
	assert.False(t, second.resetCalled, "Second parser should not be activated")
}

// ---------------------------------------------------------------------------
// Registry and NewStackTraceAggregatorFromNames tests
// ---------------------------------------------------------------------------

func TestNewStackTraceAggregatorFromNames_Go(t *testing.T) {
	agg := NewStackTraceAggregatorFromNames([]string{"go"}, testMaxContentSize)
	// Should behave like a real aggregator: buffer a Go panic
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg(""))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("goroutine 1 [running]:"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("main.main()"))
	assert.Empty(t, out)
	out = agg.Process(makeMsg("\t/path/main.go:1 +0x1"))
	assert.Empty(t, out)
	out = agg.Flush()
	assertCombined(t, out)
}

func TestNewStackTraceAggregatorFromNames_EmptyList(t *testing.T) {
	agg := NewStackTraceAggregatorFromNames(nil, testMaxContentSize)
	// Should be a noop — everything passes through
	out := agg.Process(makeMsg("panic: bad"))
	require.Len(t, out, 1)
	assert.Equal(t, "panic: bad", string(out[0].GetContent()))
	assert.True(t, agg.IsEmpty())
}

func TestNewStackTraceAggregatorFromNames_UnknownName(t *testing.T) {
	agg := NewStackTraceAggregatorFromNames([]string{"unknown_lang"}, testMaxContentSize)
	// Unknown parser name => noop
	out := agg.Process(makeMsg("panic: bad"))
	require.Len(t, out, 1)
	assert.True(t, agg.IsEmpty())
}

func TestNewStackTraceAggregatorFromNames_MixedKnownUnknown(t *testing.T) {
	agg := NewStackTraceAggregatorFromNames([]string{"unknown_lang", "go"}, testMaxContentSize)
	// Unknown is skipped, "go" is used
	out := agg.Process(makeMsg("panic: bad"))
	assert.Empty(t, out, "Go parser should buffer this")
	out = agg.Flush()
	// Single header, chunkCount==0 -> abandoned
	assertAbandoned(t, out, 1)
}
