// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observer provides an optional hook that receives agent-internal
// logs. The logger setup attaches the handler returned by NewHandler to the
// logger it builds, so any record that passes level filtering is forwarded to
// the observer registered with SetLogObserver.
package observer

import (
	"context"
	"log/slog"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
	"go.uber.org/atomic"
)

// LogObserver is an optional hook that receives agent-internal logs (already formatted and scrubbed)
// after level filtering but before they are written to the underlying logger.
//
// Observers MUST be fast and MUST NOT block. Implementations should treat this callback as best-effort.
type LogObserver func(level types.LogLevel, message string)

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

// SimulateLogEmit invokes the registered LogObserver directly (if any).
// Intended for tests that need to trigger the observer without going through
// the logger.
func SimulateLogEmit(level types.LogLevel, message string) {
	observe(level, message)
}

// NewHandler returns a slog handler that forwards the records it receives to
// the observer registered with SetLogObserver, if any, and drops them
// otherwise. Attach it downstream of the async handler — inside the
// multi-handler it drains — so the observer callback is invoked by the
// drain loop, one record at a time: it never runs concurrently with itself,
// and a record is observed before it is written.
func NewHandler() slog.Handler {
	return &handler{}
}

type handler struct{}

// Enabled returns true if an observer is registered, so the multi-handler
// short-circuits observation entirely while there is none.
func (h *handler) Enabled(_ context.Context, _ slog.Level) bool {
	return logObserverHook.Load() != nil
}

// Handle forwards the record to the registered observer.
func (h *handler) Handle(_ context.Context, r slog.Record) error {
	observe(fromSlogLevel(r.Level), r.Message)
	return nil
}

// WithAttrs returns the same handler: observation is not configurable per
// attribute group.
func (h *handler) WithAttrs([]slog.Attr) slog.Handler { return h }

// WithGroup returns the same handler: observation is not configurable per
// group.
func (h *handler) WithGroup(string) slog.Handler { return h }

func observe(level types.LogLevel, message string) {
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

// fromSlogLevel converts a slog level to a LogLevel, clamping to the closest
// known level. Records can carry levels outside the agent's set (e.g.
// slog.LevelInfo+2 from slog callers), and observing them must not panic.
func fromSlogLevel(level slog.Level) types.LogLevel {
	switch {
	case level <= slog.Level(types.TraceLvl):
		return types.TraceLvl
	case level <= slog.Level(types.DebugLvl):
		return types.DebugLvl
	case level <= slog.Level(types.InfoLvl):
		return types.InfoLvl
	case level <= slog.Level(types.WarnLvl):
		return types.WarnLvl
	case level <= slog.Level(types.ErrorLvl):
		return types.ErrorLvl
	case level <= slog.Level(types.CriticalLvl):
		return types.CriticalLvl
	default:
		return types.Off
	}
}
