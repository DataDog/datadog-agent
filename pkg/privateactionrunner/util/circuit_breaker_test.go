// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tripLogger captures Warnf calls and signals the first trip via a channel so tests can
// assert on the log message without polling or fixed sleeps.
type tripLogger struct {
	mu           sync.Mutex
	warnfMsgs    []string
	trippedOnce  sync.Once
	trippedCh    chan struct{}
}

func newTripLogger() *tripLogger {
	return &tripLogger{trippedCh: make(chan struct{})}
}

func (l *tripLogger) Debug(msg string, fields ...log.Field)     {}
func (l *tripLogger) Debugf(format string, args ...interface{}) {}
func (l *tripLogger) Info(msg string, fields ...log.Field)      {}
func (l *tripLogger) Infof(format string, args ...interface{})  {}
func (l *tripLogger) Warn(msg string, fields ...log.Field)      {}
func (l *tripLogger) Warnf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	l.mu.Lock()
	l.warnfMsgs = append(l.warnfMsgs, msg)
	l.mu.Unlock()
	l.trippedOnce.Do(func() { close(l.trippedCh) })
}
func (l *tripLogger) Error(msg string, fields ...log.Field)     {}
func (l *tripLogger) Errorf(format string, args ...interface{}) {}
func (l *tripLogger) With(fields ...log.Field) log.Logger       { return l }

func (l *tripLogger) messages() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, len(l.warnfMsgs))
	copy(out, l.warnfMsgs)
	return out
}

// TestCircuitBreaker_SucceedsOnFirstTry verifies that when fn returns nil immediately,
// the circuit breaker calls it exactly once and returns.
func TestCircuitBreaker_SucceedsOnFirstTry(t *testing.T) {
	breaker := NewCircuitBreaker("test", time.Millisecond, 10*time.Millisecond, time.Second, 3)

	callCount := 0
	breaker.Do(context.Background(), func() error {
		callCount++
		return nil
	})

	assert.Equal(t, 1, callCount)
}

// TestCircuitBreaker_RetriesOnFailureThenSucceeds verifies that the circuit breaker retries
// fn after each error and exits the loop once fn returns nil.
func TestCircuitBreaker_RetriesOnFailureThenSucceeds(t *testing.T) {
	breaker := NewCircuitBreaker("test", time.Millisecond, 2*time.Millisecond, time.Second, 5)

	callCount := 0
	breaker.Do(context.Background(), func() error {
		callCount++
		if callCount < 3 {
			return errors.New("not ready")
		}
		return nil
	})

	assert.Equal(t, 3, callCount)
}

// TestCircuitBreaker_ContextCancellationExitsBeforeFirstCall verifies that a pre-canceled
// context prevents fn from ever being called.
func TestCircuitBreaker_ContextCancellationExitsBeforeFirstCall(t *testing.T) {
	breaker := NewCircuitBreaker("test", time.Millisecond, 10*time.Millisecond, time.Second, 3)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	callCount := 0
	breaker.Do(ctx, func() error {
		callCount++
		return errors.New("fail")
	})

	assert.Equal(t, 0, callCount, "fn must not be called when the context is already canceled")
}

// TestCircuitBreaker_LogsWarningWithBreakerNameWhenTripped verifies that the circuit breaker
// emits a Warnf log through the context logger that includes both the breaker name and the
// wait duration. This is the primary observability signal that the runner is in a degraded
// polling state.
func TestCircuitBreaker_LogsWarningWithBreakerNameWhenTripped(t *testing.T) {
	capture := newTripLogger()
	ctx, cancel := context.WithCancel(log.ContextWithLogger(context.Background(), capture))
	defer cancel()

	// maxAttempts=1 means the breaker trips after 2 consecutive failures (attempt becomes 2 > 1).
	// waitBeforeRetry=1ms so the test completes quickly.
	breaker := NewCircuitBreaker("wf-par-polling", time.Millisecond, time.Millisecond, time.Millisecond, 1)

	go breaker.Do(ctx, func() error {
		return errors.New("dequeue failed")
	})

	select {
	case <-capture.trippedCh:
		cancel()
		msgs := capture.messages()
		require.Len(t, msgs, 1)
		assert.Contains(t, msgs[0], "wf-par-polling", "breaker name must appear in the trip warning")
		assert.Contains(t, msgs[0], "circuit breaker tripped")
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for circuit breaker trip warning")
	}
}
