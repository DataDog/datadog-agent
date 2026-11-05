// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/slog/handlers"
	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

func TestSetLogObserver(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	var calls []string
	SetLogObserver(func(level types.LogLevel, message string) {
		calls = append(calls, level.String()+":"+message)
	})

	observe(types.InfoLvl, "hello")
	observe(types.WarnLvl, "world")
	observe(types.ErrorLvl, "err")

	require.Len(t, calls, 3)
	assert.Equal(t, "info:hello", calls[0])
	assert.Equal(t, "warn:world", calls[1])
	assert.Equal(t, "error:err", calls[2])

	// Nil disables
	SetLogObserver(nil)
	observe(types.InfoLvl, "after-nil")
	assert.Len(t, calls, 3)
}

func TestLogObserverRecursionGuard(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	var count atomic.Int32
	SetLogObserver(func(_ types.LogLevel, _ string) {
		count.Add(1)
		// Simulate recursive log emission from within the observer.
		observe(types.DebugLvl, "recursive")
	})

	observe(types.InfoLvl, "trigger")
	// Should be exactly 1: the recursive call is swallowed by the guard.
	assert.Equal(t, int32(1), count.Load())
}

func TestLoggerName(t *testing.T) {
	t.Cleanup(func() { loggerName.Store("") })

	assert.Empty(t, GetLoggerName())
	SetLoggerName("CORE")
	assert.Equal(t, "core", GetLoggerName())
	SetLoggerName("")
	assert.Empty(t, GetLoggerName())
}

func TestHandler(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	h := NewHandler()

	// no observer registered: the multi-handler short-circuits
	assert.False(t, h.Enabled(context.Background(), slog.LevelInfo))

	var calls []string
	SetLogObserver(func(level types.LogLevel, message string) {
		calls = append(calls, level.String()+":"+message)
	})
	assert.True(t, h.Enabled(context.Background(), slog.LevelInfo))

	r := slog.NewRecord(time.Now(), slog.LevelInfo, "hello", 0)
	require.NoError(t, h.Handle(context.Background(), r))

	require.Len(t, calls, 1)
	assert.Equal(t, "info:hello", calls[0])
}

func TestHandlerConcurrentEmissions(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	var mu sync.Mutex
	var messages []string
	SetLogObserver(func(_ types.LogLevel, message string) {
		mu.Lock()
		defer mu.Unlock()
		messages = append(messages, message)
	})

	// the observer handler runs inside the multi-handler drained by the
	// async handler, like in the logger setup: records are observed one at
	// a time by the drain loop, so concurrent emissions from several
	// goroutines are not mistaken for recursion and dropped
	async := handlers.NewAsync(handlers.NewMulti(NewHandler()))
	t.Cleanup(async.Close)

	const goroutines, perGoroutine = 8, 50
	var wg sync.WaitGroup
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range perGoroutine {
				r := slog.NewRecord(time.Now(), slog.LevelInfo, fmt.Sprintf("message %d-%d", i, j), 0)
				assert.NoError(t, async.Handle(context.Background(), r))
			}
		}()
	}
	wg.Wait()
	async.Flush()

	require.Len(t, messages, goroutines*perGoroutine, "no concurrent emission may be dropped")
}
