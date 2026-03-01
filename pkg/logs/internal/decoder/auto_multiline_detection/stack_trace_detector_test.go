// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStackTraceDetector(t *testing.T) {
	detector := NewStackTraceDetector()

	testCases := []struct {
		name           string
		rawMessage     string
		expectedLabel  Label
		expectedResult bool
	}{
		// Go panic start markers
		{"go goroutine running", "goroutine 1 [running]:", startGroup, false},
		{"go goroutine many digits", "goroutine 12345 [running]:", startGroup, false},
		{"go goroutine chan receive", "goroutine 42 [chan receive]:", startGroup, false},
		{"go panic prefix", "panic: runtime error: index out of range [0] with length 0", startGroup, false},
		{"go panic nil pointer", "panic: runtime error: invalid memory address or nil pointer dereference", startGroup, false},
		{"go panic custom message", "panic: something went wrong", startGroup, false},

		// Java/Kotlin UncaughtExceptionHandler
		{"java exception in thread main", `Exception in thread "main" java.lang.NullPointerException`, startGroup, false},
		{"java exception in thread custom", `Exception in thread "pool-1-thread-1" java.lang.RuntimeException: fail`, startGroup, false},
		{"java exception in thread bare", "Exception in thread", startGroup, false},

		// Rust panic
		{"rust thread panic pre-1.73", "thread 'main' panicked at 'index out of bounds: the len is 3 but the index is 5', src/main.rs:4:5", startGroup, false},
		{"rust thread panic post-1.73", "thread 'main' panicked at src/main.rs:4:5:", startGroup, false},
		{"rust thread panic custom worker", "thread 'tokio-runtime-worker' panicked at 'called `Result::unwrap()` on an `Err` value'", startGroup, false},
		{"rust thread panic short name", "thread 'a' panicked at src/lib.rs:1:1", startGroup, false},

		// Non-matching lines (should pass through with default aggregate)
		{"normal log line", "2024-03-28 13:45:30 INFO Starting server", aggregate, true},
		{"empty line", "", aggregate, true},
		{"plain text", "Hello world", aggregate, true},
		{"json line", `{"key": "value"}`, aggregate, true},
		{"indented line", "    some indented content", aggregate, true},
		{"tab indented line", "\tat some.method()", aggregate, true},
		{"hash comment", "# this is a comment", aggregate, true},
		{"number sign", "#include <stdio.h>", aggregate, true},

		// False positive rejection: words starting with same letters
		{"goroutine word not panic", "goroutines are lightweight threads", aggregate, true},
		{"goroutine pool", "goroutine pool size: 10", aggregate, true},
		{"goroutine no digit", "goroutine [running]:", aggregate, true},
		{"goroutine space only", "goroutine ", aggregate, true},

		// Goroutine false positive rejection: digit present but no " [" status bracket
		{"goroutine started", "goroutine 5 started processing batch", aggregate, true},
		{"goroutine handling", "goroutine 42 handling request from client", aggregate, true},
		{"goroutine count", "goroutine 2048 count exceeds threshold", aggregate, true},
		{"goroutine digit only", "goroutine 1", aggregate, true},
		{"goroutine digit space no bracket", "goroutine 1 running", aggregate, true},
		{"exception word", "Exceptions should be handled properly", aggregate, true},
		{"thread word", "threads are running concurrently", aggregate, true},
		{"thread no quote", "thread main panicked", aggregate, true},
		{"panic no colon", "panicking is bad practice", aggregate, true},
		{"panic no space", "panic!", aggregate, true},

		// Rust false positive rejection: thread lifecycle logs that are NOT panics
		{"thread processing", "thread 'worker-5' processing request", aggregate, true},
		{"thread spawned", "thread 'tokio-runtime-worker' spawned", aggregate, true},
		{"thread started", "thread 'main' started successfully", aggregate, true},
		{"thread timed out", "thread 'pool-1' timed out waiting for task", aggregate, true},
		{"thread initialized", "thread 'main' initialized", aggregate, true},
		{"thread no closing quote", "thread 'this has no closing quote and goes on for a long time", aggregate, true},
		{"thread bare prefix", "thread '", aggregate, true},

		// Stack trace continuation lines (should remain default aggregate -- NOT our job)
		{"java frame", "\tat com.example.MyClass.doSomething(MyClass.java:42)", aggregate, true},
		{"python frame", `  File "/app/main.py", line 10, in main`, aggregate, true},
		{"node frame", "    at processItems (/app/src/handler.js:25:18)", aggregate, true},
		{"go frame", "\t/home/user/project/main.go:12 +0x1c0", aggregate, true},
		{"caused by", "Caused by: java.io.IOException: Stream closed", aggregate, true},
		{"rust frame", "   0: rust_begin_unwind", aggregate, true},

		// Excluded patterns (must NOT match -- would cause regressions)
		{"python traceback", "Traceback (most recent call last):", aggregate, true},
		{"php fatal error", "Fatal error: Uncaught TypeError", aggregate, true},
		{"linux kernel bug", "BUG: unable to handle page fault", aggregate, true},
		{"linux call trace", "Call Trace:", aggregate, true},
		{"android fatal", "FATAL EXCEPTION: main", aggregate, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &messageContext{
				rawMessage:      []byte(tc.rawMessage),
				label:           aggregate,
				labelAssignedBy: defaultLabelSource,
			}
			result := detector.ProcessAndContinue(ctx)
			assert.Equal(t, tc.expectedResult, result, "ProcessAndContinue return value")
			assert.Equal(t, tc.expectedLabel, ctx.label, "label")
		})
	}
}

func TestStackTraceDetectorDoesntOverrideAssignedLabel(t *testing.T) {
	detector := NewStackTraceDetector()

	testCases := []struct {
		name            string
		rawMessage      string
		priorLabel      Label
		priorAssignedBy string
	}{
		{"timestamp already set startGroup", "goroutine 1 [running]:", startGroup, "timestamp_detector"},
		{"user sample already set startGroup", "panic: something", startGroup, "user_sample"},
		{"json detector set noAggregate", "goroutine 1 [running]:", noAggregate, "JSON_detector"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &messageContext{
				rawMessage:      []byte(tc.rawMessage),
				label:           tc.priorLabel,
				labelAssignedBy: tc.priorAssignedBy,
			}
			result := detector.ProcessAndContinue(ctx)
			assert.True(t, result, "should continue processing when label already assigned")
			assert.Equal(t, tc.priorLabel, ctx.label, "should not change the label")
			assert.Equal(t, tc.priorAssignedBy, ctx.labelAssignedBy, "should not change labelAssignedBy")
		})
	}
}

func TestStackTraceDetectorLabelAssignedBy(t *testing.T) {
	detector := NewStackTraceDetector()

	ctx := &messageContext{
		rawMessage:      []byte("goroutine 1 [running]:"),
		label:           aggregate,
		labelAssignedBy: defaultLabelSource,
	}
	detector.ProcessAndContinue(ctx)
	assert.Equal(t, "stack_trace_detector", ctx.labelAssignedBy)
}
