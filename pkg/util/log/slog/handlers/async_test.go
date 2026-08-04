// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package handlers

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// recordCapture is a helper to capture records from the format handler
type recordCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (rc *recordCapture) formatter(_ context.Context, r slog.Record) string {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.records = append(rc.records, r)
	return ""
}

func (rc *recordCapture) getRecords() []slog.Record {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	return append([]slog.Record{}, rc.records...)
}

// collectAttrs collects all attributes from a record into a slice
func collectAttrs(r slog.Record) []slog.Attr {
	var attrs []slog.Attr
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	return attrs
}

func TestAsyncHandlerWithAttrs(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	// Create a derived handler with attributes
	derivedHandler := handler.WithAttrs([]slog.Attr{
		slog.String("key1", "value1"),
		slog.Int("key2", 42),
	})

	// Log through the derived handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test with attrs", 0)
	err := derivedHandler.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Flush using the original handler (shared state)
	handler.Flush()

	// Verify the record was logged with the correct attributes
	records := capture.getRecords()
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "test with attrs", records[0].Message)

	attrs := collectAttrs(records[0])
	assert.Equal(t, 2, len(attrs))
	assert.Equal(t, "key1", attrs[0].Key)
	assert.Equal(t, "value1", attrs[0].Value.String())
	assert.Equal(t, "key2", attrs[1].Key)
	assert.Equal(t, int64(42), attrs[1].Value.Int64())
}

func TestAsyncHandlerWithGroup(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	// Create a derived handler with a group
	derivedHandler := handler.WithGroup("mygroup")

	// Log through the derived handler with an attribute
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test with group", 0)
	record.AddAttrs(slog.String("nested", "value"))
	err := derivedHandler.Handle(context.Background(), record)
	assert.NoError(t, err)

	// Flush using the original handler (shared state)
	handler.Flush()

	// Verify the record was logged with the attribute nested in the group
	records := capture.getRecords()
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "test with group", records[0].Message)

	attrs := collectAttrs(records[0])
	assert.Equal(t, 1, len(attrs))
	assert.Equal(t, "mygroup", attrs[0].Key)
	assert.Equal(t, slog.KindGroup, attrs[0].Value.Kind())

	groupAttrs := attrs[0].Value.Group()
	assert.Equal(t, 1, len(groupAttrs))
	assert.Equal(t, "nested", groupAttrs[0].Key)
	assert.Equal(t, "value", groupAttrs[0].Value.String())
}

func TestAsyncHandlerWithAttrsAndGroup(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	// Chain WithAttrs and WithGroup
	derivedHandler := handler.
		WithAttrs([]slog.Attr{slog.String("attr1", "val1")}).
		WithGroup("group1").
		WithAttrs([]slog.Attr{slog.String("attr2", "val2")})

	// Log through the derived handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "chained", 0)
	err := derivedHandler.Handle(context.Background(), record)
	assert.NoError(t, err)

	handler.Flush()

	records := capture.getRecords()
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "chained", records[0].Message)

	// Verify: attr1 at top level, group1 containing attr2
	attrs := collectAttrs(records[0])
	assert.Equal(t, 2, len(attrs))
	assert.Equal(t, "attr1", attrs[0].Key)
	assert.Equal(t, "val1", attrs[0].Value.String())
	assert.Equal(t, "group1", attrs[1].Key)

	groupAttrs := attrs[1].Value.Group()
	assert.Equal(t, 1, len(groupAttrs))
	assert.Equal(t, "attr2", groupAttrs[0].Key)
	assert.Equal(t, "val2", groupAttrs[0].Value.String())
}

func TestAsyncHandlerWithAttrsSharesState(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	// Create derived handlers
	derivedHandler1 := handler.WithAttrs([]slog.Attr{slog.String("handler", "1")})
	derivedHandler2 := handler.WithAttrs([]slog.Attr{slog.String("handler", "2")})

	// Log through different handlers
	record1 := slog.NewRecord(time.Now(), slog.LevelInfo, "message 1", 0)
	record2 := slog.NewRecord(time.Now(), slog.LevelInfo, "message 2", 0)

	derivedHandler1.Handle(context.Background(), record1)
	derivedHandler2.Handle(context.Background(), record2)

	// Flush through original handler should flush all messages
	handler.Flush()

	records := capture.getRecords()
	assert.Equal(t, 2, len(records))

	// Check that each message has the correct attributes
	assert.Equal(t, "message 1", records[0].Message)
	attrs1 := collectAttrs(records[0])
	assert.Equal(t, "1", attrs1[0].Value.String())

	assert.Equal(t, "message 2", records[1].Message)
	attrs2 := collectAttrs(records[1])
	assert.Equal(t, "2", attrs2[0].Value.String())
}

func TestAsyncHandlerWithGroupSharesState(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	// Create derived handlers with different groups
	derivedHandler1 := handler.WithGroup("group1")
	derivedHandler2 := handler.WithGroup("group2")

	// Log through different handlers with attributes
	record1 := slog.NewRecord(time.Now(), slog.LevelInfo, "message 1", 0)
	record1.AddAttrs(slog.String("key", "val1"))
	record2 := slog.NewRecord(time.Now(), slog.LevelInfo, "message 2", 0)
	record2.AddAttrs(slog.String("key", "val2"))

	derivedHandler1.Handle(context.Background(), record1)
	derivedHandler2.Handle(context.Background(), record2)

	// Flush through original handler should flush all messages
	handler.Flush()

	records := capture.getRecords()
	assert.Equal(t, 2, len(records))

	// Check that each message has the correct group
	assert.Equal(t, "message 1", records[0].Message)
	attrs1 := collectAttrs(records[0])
	assert.Equal(t, "group1", attrs1[0].Key)

	assert.Equal(t, "message 2", records[1].Message)
	attrs2 := collectAttrs(records[1])
	assert.Equal(t, "group2", attrs2[0].Key)
}

func TestAsyncHandlerDerivedHandlerClose(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)

	// Create a derived handler
	derivedHandler := handler.WithAttrs([]slog.Attr{slog.String("key", "value")})

	// Log through derived handler
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "before close", 0)
	derivedHandler.Handle(context.Background(), record)

	// Close the original handler
	handler.Close()

	// Message should have been processed
	records := capture.getRecords()
	assert.Equal(t, 1, len(records))
	assert.Equal(t, "before close", records[0].Message)

	// Logging after close should be ignored (through derived handler)
	record2 := slog.NewRecord(time.Now(), slog.LevelInfo, "after close", 0)
	derivedHandler.Handle(context.Background(), record2)

	// Give time to ensure nothing is processed
	time.Sleep(10 * time.Millisecond)

	records = capture.getRecords()
	assert.Equal(t, 1, len(records))
}

// TestAsyncHandlerSnapshotsAnyAttrsImmediately checks that a KindAny value is snapshotted
// before Handle() returns, so a mutation right after doesn't leak into the logged output.
func TestAsyncHandlerSnapshotsAnyAttrsImmediately(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	type mutable struct {
		Field string
	}
	obj := &mutable{Field: "before"}

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.Any("obj", obj))
	require.NoError(t, handler.Handle(context.Background(), record))

	obj.Field = "after"

	handler.Flush()

	records := capture.getRecords()
	require.Len(t, records, 1)
	attrs := collectAttrs(records[0])
	require.Len(t, attrs, 1)
	assert.Equal(t, slog.KindString, attrs[0].Value.Kind())
	assert.Equal(t, fmt.Sprint(&mutable{Field: "before"}), attrs[0].Value.String())
}

func TestAsyncHandlerLeavesSafeAttrsUnchanged(t *testing.T) {
	capture := &recordCapture{}
	inner := NewFormat(capture.formatter, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
	record.AddAttrs(slog.Int("count", 42), slog.String("name", "foo"))
	require.NoError(t, handler.Handle(context.Background(), record))

	handler.Flush()

	records := capture.getRecords()
	require.Len(t, records, 1)
	attrs := collectAttrs(records[0])
	require.Len(t, attrs, 2)
	assert.Equal(t, slog.KindInt64, attrs[0].Value.Kind())
	assert.Equal(t, int64(42), attrs[0].Value.Int64())
	assert.Equal(t, slog.KindString, attrs[1].Value.Kind())
	assert.Equal(t, "foo", attrs[1].Value.String())
}

// TestAsyncHandlerHandleRaceWithMutation reproduces the production race: log a mutable
// object then mutate it right after, while the background goroutine formats it. Run with
// -race.
func TestAsyncHandlerHandleRaceWithMutation(t *testing.T) {
	inner := NewFormat(func(_ context.Context, r slog.Record) string {
		var sb strings.Builder
		r.Attrs(func(a slog.Attr) bool {
			sb.WriteString(a.Value.String())
			return true
		})
		return sb.String()
	}, io.Discard)
	handler := NewAsync(inner)
	defer handler.Close()

	type mutable struct {
		Field string
	}

	for i := range 1000 {
		obj := &mutable{Field: "before"}
		record := slog.NewRecord(time.Now(), slog.LevelInfo, "test", 0)
		record.AddAttrs(slog.Any("obj", obj))
		require.NoError(t, handler.Handle(context.Background(), record))

		obj.Field = fmt.Sprintf("after-%d", i)
	}

	handler.Flush()
}
