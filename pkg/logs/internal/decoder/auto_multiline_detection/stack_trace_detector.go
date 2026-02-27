// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import "bytes"

// maxThreadNameLen is the maximum number of bytes to scan past "thread '" when
// looking for the closing quote. Real Rust thread names are rarely longer than ~40 chars.
const maxThreadNameLen = 64

var (
	prefixGoroutine       = []byte("goroutine ")
	prefixPanic           = []byte("panic: ")
	prefixExceptionThread = []byte("Exception in thread")
	prefixRustThreadPanic = []byte("thread '")
	suffixRustPanicked    = []byte("' panicked")
)

// StackTraceDetector is a heuristic that detects the start of stack traces that lack timestamps.
//
// It only recognizes patterns that are exclusively emitted by language runtimes writing directly
// to stderr (Go panics, Rust panics, Java's UncaughtExceptionHandler). Patterns that can
// legitimately appear as continuation lines after a timestamped log entry (e.g. Python's
// "Traceback") are intentionally excluded to avoid severing timestamped entries from their traces.
type StackTraceDetector struct{}

// NewStackTraceDetector returns a new StackTraceDetector heuristic.
func NewStackTraceDetector() *StackTraceDetector {
	return &StackTraceDetector{}
}

// ProcessAndContinue checks if a message is the start of a stack trace.
// It only acts on lines that have not been labeled by a prior heuristic.
// On match, it labels the line as startGroup and short-circuits (returns false).
func (s *StackTraceDetector) ProcessAndContinue(context *messageContext) bool {
	if context.labelAssignedBy != defaultLabelSource {
		return true
	}

	msg := context.rawMessage
	if len(msg) == 0 {
		return true
	}

	switch msg[0] {
	case 'g':
		if bytes.HasPrefix(msg, prefixGoroutine) && isGoGoroutine(msg) {
			context.label = startGroup
			context.labelAssignedBy = "stack_trace_detector"
			return false
		}
	case 'p':
		if bytes.HasPrefix(msg, prefixPanic) {
			context.label = startGroup
			context.labelAssignedBy = "stack_trace_detector"
			return false
		}
	case 'E':
		if bytes.HasPrefix(msg, prefixExceptionThread) {
			context.label = startGroup
			context.labelAssignedBy = "stack_trace_detector"
			return false
		}
	case 't':
		if bytes.HasPrefix(msg, prefixRustThreadPanic) && isRustPanic(msg) {
			context.label = startGroup
			context.labelAssignedBy = "stack_trace_detector"
			return false
		}
	}

	return true
}

// isGoGoroutine checks for the Go runtime goroutine format: goroutine N [status]:
// After "goroutine ", verifies digit(s) followed by " [" which is the runtime's
// status bracket. This rejects non-panic lines like "goroutine 5 started processing".
func isGoGoroutine(msg []byte) bool {
	i := len(prefixGoroutine)
	if i >= len(msg) || !isDigit(msg[i]) {
		return false
	}
	i++
	for i < len(msg) && isDigit(msg[i]) {
		i++
	}
	return i+1 < len(msg) && msg[i] == ' ' && msg[i+1] == '['
}

// isRustPanic checks for the full Rust panic pattern: thread '<name>' panicked
// by scanning for the closing quote within a bounded window and verifying
// "' panicked" follows it. This eliminates false positives from non-panic
// thread-related log lines like "thread 'worker' processing request".
func isRustPanic(msg []byte) bool {
	nameStart := len(prefixRustThreadPanic)
	end := min(len(msg), nameStart+maxThreadNameLen)
	closeQuote := bytes.IndexByte(msg[nameStart:end], '\'')
	if closeQuote < 0 {
		return false
	}
	return bytes.HasPrefix(msg[nameStart+closeQuote:], suffixRustPanicked)
}

func isDigit(b byte) bool {
	return b >= '0' && b <= '9'
}
