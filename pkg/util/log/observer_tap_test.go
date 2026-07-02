// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetLogObserver(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	var calls []string
	SetLogObserver(func(level LogLevel, message string) {
		calls = append(calls, level.String()+":"+message)
	})

	maybeObserve(InfoLvl, "hello")
	maybeObserve(WarnLvl, "world")
	maybeObserve(ErrorLvl, "err")

	require.Len(t, calls, 3)
	assert.Equal(t, "info:hello", calls[0])
	assert.Equal(t, "warn:world", calls[1])
	assert.Equal(t, "error:err", calls[2])

	// Nil disables
	SetLogObserver(nil)
	maybeObserve(InfoLvl, "after-nil")
	assert.Len(t, calls, 3)
}

func TestLogObserverRecursionGuard(t *testing.T) {
	t.Cleanup(func() { SetLogObserver(nil) })

	var count atomic.Int32
	SetLogObserver(func(_ LogLevel, _ string) {
		count.Add(1)
		// Simulate recursive log emission from within the observer.
		maybeObserve(DebugLvl, "recursive")
	})

	maybeObserve(InfoLvl, "trigger")
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
