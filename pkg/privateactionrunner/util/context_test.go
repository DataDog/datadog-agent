// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"testing"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogger captures structured log calls so tests can assert on them.
type captureLogger struct {
	warnMessages  []string
	warnFields    [][]log.Field
}

func (c *captureLogger) Debug(msg string, fields ...log.Field)     {}
func (c *captureLogger) Debugf(format string, args ...interface{}) {}
func (c *captureLogger) Info(msg string, fields ...log.Field)      {}
func (c *captureLogger) Infof(format string, args ...interface{})  {}
func (c *captureLogger) Warn(msg string, fields ...log.Field) {
	c.warnMessages = append(c.warnMessages, msg)
	c.warnFields = append(c.warnFields, fields)
}
func (c *captureLogger) Warnf(format string, args ...interface{}) {}
func (c *captureLogger) Error(msg string, fields ...log.Field)    {}
func (c *captureLogger) Errorf(format string, args ...interface{}) {}
func (c *captureLogger) With(fields ...log.Field) log.Logger       { return c }

// TestCreateTimeoutContext_NilTimeout verifies that a nil timeout returns the original context
// unchanged (i.e., no deadline is attached).
func TestCreateTimeoutContext_NilTimeout(t *testing.T) {
	ctx := context.Background()
	newCtx, cancel := CreateTimeoutContext(ctx, nil)
	defer cancel()

	assert.Equal(t, ctx, newCtx, "should return the original context when timeout is nil")
	_, hasDeadline := newCtx.Deadline()
	assert.False(t, hasDeadline, "context must not have a deadline when timeout is nil")
}

// TestCreateTimeoutContext_ZeroTimeout verifies that a zero timeout is treated the same as nil:
// no deadline is attached and the original context is returned.
func TestCreateTimeoutContext_ZeroTimeout(t *testing.T) {
	ctx := context.Background()
	zero := int32(0)
	newCtx, cancel := CreateTimeoutContext(ctx, &zero)
	defer cancel()

	assert.Equal(t, ctx, newCtx, "should return the original context when timeout is 0")
	_, hasDeadline := newCtx.Deadline()
	assert.False(t, hasDeadline, "context must not have a deadline when timeout is 0")
}

// TestCreateTimeoutContext_PositiveTimeout verifies that a positive timeout attaches a deadline
// to the returned context approximately equal to now + timeout seconds.
func TestCreateTimeoutContext_PositiveTimeout(t *testing.T) {
	ctx := context.Background()
	timeout := int32(5)
	newCtx, cancel := CreateTimeoutContext(ctx, &timeout)
	defer cancel()

	deadline, hasDeadline := newCtx.Deadline()
	require.True(t, hasDeadline, "context must have a deadline when timeout > 0")
	assert.WithinDuration(t, time.Now().Add(5*time.Second), deadline, time.Second,
		"deadline should be ~5 seconds from now")
}

// TestHandleTimeoutError_DeadlineExceededLogsWarning verifies that when the context's
// deadline has been exceeded and an error is present, HandleTimeoutError:
//   - returns isTimeout=true
//   - returns an error mentioning the configured timeout duration
//   - logs exactly one Warn with the timeout message
func TestHandleTimeoutError_DeadlineExceededLogsWarning(t *testing.T) {
	timeout := int32(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	// Let the context expire before calling.
	time.Sleep(10 * time.Millisecond)

	logger := &captureLogger{}
	isTimeout, err := HandleTimeoutError(ctx, errors.New("some inner error"), &timeout, logger)

	assert.True(t, isTimeout)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "1 seconds")

	require.Len(t, logger.warnMessages, 1, "exactly one Warn should be emitted")
	assert.Contains(t, logger.warnMessages[0], "timed out")

	// Verify that the timeout_seconds field is included in the structured fields.
	require.Len(t, logger.warnFields[0], 1)
	assert.Equal(t, "timeout_seconds", logger.warnFields[0][0].Key)
	assert.Equal(t, int32(1), logger.warnFields[0][0].Value)
}

// TestHandleTimeoutError_ContextNotExpired verifies that when the context is still valid,
// HandleTimeoutError returns (false, nil) and does not log anything.
func TestHandleTimeoutError_ContextNotExpired(t *testing.T) {
	timeout := int32(30)
	ctx := context.Background()
	logger := &captureLogger{}

	isTimeout, err := HandleTimeoutError(ctx, errors.New("some error"), &timeout, logger)

	assert.False(t, isTimeout)
	assert.Nil(t, err)
	assert.Empty(t, logger.warnMessages, "no warning should be logged when context is still valid")
}

// TestHandleTimeoutError_NilErrWithExpiredContext verifies that if the context has expired
// but the error argument is nil, HandleTimeoutError treats this as a non-timeout condition
// (the caller had no error, so no timeout-related handling is needed).
func TestHandleTimeoutError_NilErrWithExpiredContext(t *testing.T) {
	timeout := int32(1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()

	time.Sleep(10 * time.Millisecond)

	logger := &captureLogger{}
	isTimeout, err := HandleTimeoutError(ctx, nil, &timeout, logger)

	assert.False(t, isTimeout, "nil error should not be classified as a timeout")
	assert.Nil(t, err)
	assert.Empty(t, logger.warnMessages)
}
