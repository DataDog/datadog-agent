// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"strings"

	"go.uber.org/atomic"
)

// LogObserver is an optional hook that receives agent-internal logs (already formatted and scrubbed)
// after level filtering but before they are written to the underlying logger.
//
// Observers MUST be fast and MUST NOT block. Implementations should treat this callback as best-effort.
type LogObserver func(level LogLevel, message string)

var (
	logObserverHook atomic.Pointer[LogObserver]
	// observing guards against accidental recursion if an observer emits logs.
	observing atomic.Bool

	loggerName atomic.String
)

// SetLogObserver registers a process-wide log observer hook.
// Passing nil disables observation.
func SetLogObserver(h LogObserver) {
	if h == nil {
		logObserverHook.Store(nil)
		return
	}
	hp := new(LogObserver)
	*hp = h
	logObserverHook.Store(hp)
}

// SetLoggerName records the current logger name (e.g. CORE, DOGSTATSD) for low-cardinality tagging.
// This is optional; if unset, GetLoggerName returns "".
func SetLoggerName(name string) {
	loggerName.Store(strings.ToLower(name))
}

// GetLoggerName returns the recorded logger name (if any).
func GetLoggerName() string {
	return loggerName.Load()
}

func maybeObserve(level LogLevel, message string) {
	hp := logObserverHook.Load()
	if hp == nil {
		return
	}
	// Best-effort recursion protection.
	if !observing.CompareAndSwap(false, true) {
		return
	}
	defer observing.Store(false)
	(*hp)(level, message)
}
