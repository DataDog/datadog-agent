// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package logging

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFormatMessage_NoFields verifies that a message with no fields is returned verbatim.
func TestFormatMessage_NoFields(t *testing.T) {
	l := &loggerAdapter{}
	assert.Equal(t, "hello world", l.formatMessage("hello world"))
}

// TestFormatMessage_SingleStringField verifies the "(key=value)" suffix format.
func TestFormatMessage_SingleStringField(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("task started", String("task_id", "abc123"))
	assert.Equal(t, "task started (task_id=abc123)", result)
}

// TestFormatMessage_MultipleFields verifies that multiple fields are space-separated.
func TestFormatMessage_MultipleFields(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("msg", String("a", "1"), String("b", "2"))
	assert.Equal(t, "msg (a=1 b=2)", result)
}

// TestFormatMessage_ErrorFieldOnly verifies the "msg: error" format when only an error field is present.
func TestFormatMessage_ErrorFieldOnly(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("failed to connect", ErrorField(errors.New("connection refused")))
	assert.Equal(t, "failed to connect: connection refused", result)
}

// TestFormatMessage_ErrorFieldWithOtherFields verifies the "msg: error (key=val)" format when
// both an error and other fields are present. The error always appears before the parenthetical.
func TestFormatMessage_ErrorFieldWithOtherFields(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("task failed",
		String("task_id", "xyz"),
		ErrorField(errors.New("timeout")),
	)
	assert.Equal(t, "task failed: timeout (task_id=xyz)", result)
}

// TestFormatMessage_NilErrorField verifies that a nil error passed to ErrorField is not treated
// as an error — because nil cannot be type-asserted to error from an interface{} — so it is
// formatted as a regular "error=<nil>" field.
func TestFormatMessage_NilErrorField(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("msg", ErrorField(nil))
	assert.Equal(t, "msg (error=<nil>)", result)
}

// TestFormatMessage_ContextFieldsPrecedeCallFields verifies that fields from loggerAdapter.contextFields
// (set via With) appear before any fields passed directly to the log call.
func TestFormatMessage_ContextFieldsPrecedeCallFields(t *testing.T) {
	l := &loggerAdapter{
		contextFields: []Field{String("runner_id", "runner-1")},
	}
	result := l.formatMessage("dequeued task", String("task_id", "t-42"))
	assert.Equal(t, "dequeued task (runner_id=runner-1 task_id=t-42)", result)
}

// TestFormatMessage_ContextErrorFieldWithCallFields verifies message formatting when an error
// is in the context fields and other fields are added at call time.
func TestFormatMessage_ContextErrorWithCallFields(t *testing.T) {
	l := &loggerAdapter{
		contextFields: []Field{ErrorField(errors.New("auth failed"))},
	}
	result := l.formatMessage("rejected", String("user", "agent"))
	assert.Equal(t, "rejected: auth failed (user=agent)", result)
}

// TestFormatMessage_VariousFieldTypes verifies that Int, Int64, Bool, Duration, and Any
// field types are formatted correctly via their default %v representation.
func TestFormatMessage_VariousFieldTypes(t *testing.T) {
	l := &loggerAdapter{}
	result := l.formatMessage("stats",
		Int("count", 7),
		Int64("bytes", 1024),
		Bool("healthy", true),
		Duration("elapsed", 3*time.Second),
	)
	assert.Equal(t, "stats (count=7 bytes=1024 healthy=true elapsed=3s)", result)
}

// TestWith_AccumulatesContextFields verifies that With returns a new logger whose contextFields
// include both the original fields and the newly added ones.
func TestWith_AccumulatesContextFields(t *testing.T) {
	base := &loggerAdapter{
		contextFields: []Field{String("runner_id", "r1")},
	}
	child := base.With(String("task_id", "t2"))

	la, ok := child.(*loggerAdapter)
	require.True(t, ok)
	require.Len(t, la.contextFields, 2)
	assert.Equal(t, "runner_id", la.contextFields[0].Key)
	assert.Equal(t, "task_id", la.contextFields[1].Key)
}

// TestWith_DoesNotMutateParent verifies that adding fields to a child logger via With
// does not alter the parent's contextFields.
func TestWith_DoesNotMutateParent(t *testing.T) {
	base := &loggerAdapter{
		contextFields: []Field{String("runner_id", "r1")},
	}
	_ = base.With(String("task_id", "t2"), String("action_fqn", "com.example.act"))

	assert.Len(t, base.contextFields, 1, "parent logger must not be mutated by With")
}

// TestWith_ChainedCallsAccumulateCorrectly verifies that chaining multiple With calls
// produces a logger with all accumulated fields in order.
func TestWith_ChainedCallsAccumulateCorrectly(t *testing.T) {
	base := &loggerAdapter{}
	l1 := base.With(String("a", "1"))
	l2 := l1.With(String("b", "2"))
	l3 := l2.With(String("c", "3"))

	la, ok := l3.(*loggerAdapter)
	require.True(t, ok)
	require.Len(t, la.contextFields, 3)
	assert.Equal(t, "a", la.contextFields[0].Key)
	assert.Equal(t, "b", la.contextFields[1].Key)
	assert.Equal(t, "c", la.contextFields[2].Key)
}

// TestFromContext_ReturnsNonNilWhenNoLoggerStored verifies that FromContext always returns
// a usable Logger even when no logger has been stored in the context.
func TestFromContext_ReturnsNonNilWhenNoLoggerStored(t *testing.T) {
	ctx := context.Background()
	logger := FromContext(ctx)
	assert.NotNil(t, logger)
}

// TestContextWithLogger_RoundTrip verifies that a logger stored via ContextWithLogger
// is retrieved unchanged by FromContext.
func TestContextWithLogger_RoundTrip(t *testing.T) {
	ctx := context.Background()
	stored := &loggerAdapter{contextFields: []Field{String("x", "y")}}

	ctx = ContextWithLogger(ctx, stored)
	retrieved := FromContext(ctx)

	assert.Equal(t, stored, retrieved)
}

// TestContextWithLogger_IsolatedPerContext verifies that two contexts derived from the same
// parent can carry independent loggers without interfering with each other.
func TestContextWithLogger_IsolatedPerContext(t *testing.T) {
	parent := context.Background()

	loggerA := &loggerAdapter{contextFields: []Field{String("scope", "A")}}
	loggerB := &loggerAdapter{contextFields: []Field{String("scope", "B")}}

	ctxA := ContextWithLogger(parent, loggerA)
	ctxB := ContextWithLogger(parent, loggerB)

	assert.Equal(t, loggerA, FromContext(ctxA))
	assert.Equal(t, loggerB, FromContext(ctxB))
	// Parent context remains unaffected.
	assert.NotNil(t, FromContext(parent))
	assert.NotEqual(t, loggerA, FromContext(parent))
}
