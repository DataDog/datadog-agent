// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLockingHandler_BasicFunctionality(t *testing.T) {
	mock := newMockInnerHandler()
	handler := NewLocking(mock)

	ctx := context.Background()
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)

	err := handler.Handle(ctx, record)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.recordCount())
	assert.Equal(t, "test message", mock.lastMessage())
}

func TestLockingHandler_Enabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		level   slog.Level
	}{
		{
			name:    "enabled for info",
			enabled: true,
			level:   slog.LevelInfo,
		},
		{
			name:    "disabled for debug",
			enabled: false,
			level:   slog.LevelDebug,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockInnerHandler()
			mock.enabled = tt.enabled
			handler := NewLocking(mock)

			ctx := context.Background()
			result := handler.Enabled(ctx, tt.level)
			assert.Equal(t, tt.enabled, result)
		})
	}
}

func TestLockingHandler_ErrorPropagation(t *testing.T) {
	expectedErr := errors.New("inner handler error")
	mock := newMockInnerHandler()
	mock.err = expectedErr
	handler := NewLocking(mock)

	ctx := context.Background()
	record := slog.NewRecord(time.Now(), slog.LevelError, "error message", 0)

	err := handler.Handle(ctx, record)
	assert.Equal(t, expectedErr, err)
}

func TestLockingHandler_ConcurrentAccess(t *testing.T) {
	mock := &blockingHandler{
		signal: make(chan struct{}),
	}
	handler := NewLocking(mock)

	ctx := context.Background()
	numGoroutines := 10

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch multiple goroutines that concurrently call Handle
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent message", 0)
			_ = handler.Handle(ctx, record)
		}()
	}

	time.Sleep(100 * time.Millisecond)

	// only one goroutine should have accessed the inner handler
	require.Equal(t, int32(1), mock.counter.Load())

	close(mock.signal)
	wg.Wait()

	assert.Equal(t, numGoroutines, len(mock.records))
}

func TestLockingHandler_MultipleHandleCalls(t *testing.T) {
	mock := newMockInnerHandler()
	handler := NewLocking(mock)

	ctx := context.Background()
	messages := []string{"message 1", "message 2", "message 3"}

	for _, msg := range messages {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, msg, 0)
		err := handler.Handle(ctx, record)
		require.NoError(t, err)
	}

	assert.Equal(t, len(messages), mock.recordCount())
	assert.Equal(t, messages[len(messages)-1], mock.lastMessage())
}

func TestLockingHandler_WithAttrs(t *testing.T) {
	mock := newMockInnerHandler()
	handler := NewLocking(mock).WithAttrs([]slog.Attr{slog.String("key", "value")})

	handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0))

	assert.Equal(t, 1, mock.recordCount())
	assert.Equal(t, "test", mock.lastMessage())
	mock.records[0].Attrs(func(a slog.Attr) bool {
		assert.Equal(t, "key", a.Key)
		assert.Equal(t, "value", a.Value.String())
		return true
	})
}

func TestLockingHandler_WithGroup(t *testing.T) {
	mock := newMockInnerHandler()
	handler := NewLocking(mock).WithGroup("group")

	handler.Handle(context.Background(), slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0))

	assert.Equal(t, 1, mock.recordCount())
	assert.Equal(t, "test", mock.lastMessage())
	mock.records[0].Attrs(func(a slog.Attr) bool {
		assert.Equal(t, "group", a.Key)
		return true
	})
}

func TestLockingHandler_EnabledNotSynchronized(_ *testing.T) {
	mock := newMockInnerHandler()
	handler := NewLocking(mock)

	ctx := context.Background()
	numGoroutines := 50

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Call Enabled from multiple goroutines concurrently
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_ = handler.Enabled(ctx, slog.LevelInfo)
		}()
	}

	wg.Wait()
	// Test passes if no data races are detected (run with -race flag)
}

type blockingHandler struct {
	mu      sync.Mutex
	records []slog.Record
	signal  chan struct{} // signal to unblock the handler
	counter atomic.Int32
}

func (h *blockingHandler) Enabled(_ context.Context, _ slog.Level) bool {
	return true
}

func (h *blockingHandler) Handle(_ context.Context, record slog.Record) error {
	h.counter.Add(1)
	<-h.signal

	h.mu.Lock()
	h.records = append(h.records, record)
	h.mu.Unlock()

	h.counter.Add(-1)

	return nil
}

func (h *blockingHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return h
}

func (h *blockingHandler) WithGroup(_ string) slog.Handler {
	return h
}
