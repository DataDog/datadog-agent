// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAsyncHandlerHandle(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test message", 0)
	err := handler.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Wait for async processing
	handler.Flush()

	assert.Equal(t, 1, inner.recordCount())
	assert.Equal(t, "test message", inner.lastMessage())
}

func TestAsyncHandlerMultipleMessages(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	// Send multiple messages
	for range 10 {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
		err := handler.Handle(context.Background(), record)
		assert.NoError(t, err)
	}

	// Wait for all messages to be processed
	handler.Flush()

	assert.Equal(t, 10, inner.recordCount())
}

func TestAsyncHandlerFlush(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	// Send messages
	for range 5 {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
		handler.Handle(context.Background(), record)
	}

	// Flush should wait for all messages to be written
	handler.Flush()

	assert.Equal(t, 5, inner.recordCount())

	// Multiple flushes should be safe
	handler.Flush()
	handler.Flush()

	assert.Equal(t, 5, inner.recordCount())
}

func TestAsyncHandlerClose(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	handler.Handle(context.Background(), record)

	// Close should wait for pending messages
	handler.Close()

	// Messages sent before close should be processed
	assert.Equal(t, 1, inner.recordCount())

	// Messages sent after close should be ignored
	handler.Handle(context.Background(), record)

	// Give a small amount of time to ensure no processing happens
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 1, inner.recordCount())
}

func TestAsyncHandlerEnabled(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	assert.True(t, handler.Enabled(context.Background(), slog.LevelInfo))

	inner.enabled = false
	assert.False(t, handler.Enabled(context.Background(), slog.LevelInfo))
}

func TestAsyncHandlerFlushAfterClose(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	handler.Handle(context.Background(), record)

	// Close the handler first
	handler.Close()

	// All messages should have been processed during close
	assert.Equal(t, 1, inner.recordCount())

	// Flush after close should be safe and should not panic
	handler.Flush()

	// Message count should remain the same
	assert.Equal(t, 1, inner.recordCount())

	// Multiple flushes after close should also be safe
	handler.Flush()
	handler.Flush()

	assert.Equal(t, 1, inner.recordCount())
}

func TestAsyncHandlerConcurrentWrites(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	var wg sync.WaitGroup
	messageCount := 100
	goroutines := 10

	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range messageCount {
				record := slog.NewRecord(time.Now(), slog.LevelInfo, "concurrent message", 0)
				handler.Handle(context.Background(), record)
			}
		}()
	}

	wg.Wait()
	handler.Flush()

	assert.Equal(t, messageCount*goroutines, inner.recordCount())
}

func TestAsyncHandlerConcurrentFlushes(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	// block all the routines
	handler.cond.L.Lock()

	var wg sync.WaitGroup

	// Add some messages
	wg.Add(1)
	messageCount := 100
	go func() {
		defer wg.Done()
		for range messageCount {
			record := slog.NewRecord(time.Now(), slog.LevelInfo, "message", 0)
			handler.Handle(context.Background(), record)
		}
	}()

	flushCount := 20
	wg.Add(flushCount)
	for range flushCount {
		go func() {
			defer wg.Done()
			handler.Flush()
		}()
	}

	handler.cond.L.Unlock()

	wg.Wait()
	handler.Flush()
	assert.Equal(t, messageCount, inner.recordCount())
}

// Test that empty queue flush is handled correctly
func TestAsyncHandlerFlushEmptyQueue(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	// Flush with no messages should be safe
	handler.Flush()
	assert.Equal(t, 0, inner.recordCount())
}

// Test that messages are processed in order
func TestAsyncHandlerMessageOrder(t *testing.T) {
	inner := newMockInnerHandler()
	handler := NewAsync(inner)
	defer handler.Close()

	// Send messages with different content
	for i := range 10 {
		record := slog.NewRecord(time.Now(), slog.LevelInfo, string(rune('A'+i)), 0)
		handler.Handle(context.Background(), record)
	}

	handler.Flush()

	inner.mu.Lock()
	defer inner.mu.Unlock()

	// Verify messages are in order
	for i := range 10 {
		assert.Equal(t, string(rune('A'+i)), inner.records[i].Message)
	}
}
