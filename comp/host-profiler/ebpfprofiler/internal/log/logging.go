package log

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync/atomic"
)

// programLevel controls the minimum log level for all loggers.
var programLevel = new(slog.LevelVar) // Info by default

// globalLogger holds a reference to the [slog.Logger] used within
// go.opentelemetry.io/ebpf-profiler.
//
// The default logger logs to stderr which is backed by the standard `log.Logger`
// interface. This logger will show messages at the Info Level.
var globalLogger = func() *atomic.Pointer[slog.Logger] {
	l := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: programLevel,
	}))

	p := new(atomic.Pointer[slog.Logger])
	p.Store(l)
	return p
}()

// SetLogger sets the global logger to l while respecting programLevel's log
// level. When default logger is overridden, SetLevel has no effect.
func SetLogger(l slog.Logger) {
	globalLogger.Store(&l)
}

// SetLevel dynamically changes the logger's log level, excluding
// those set via SetLogger.
func SetLevel(level slog.Level) {
	programLevel.Set(level)
}

// getLogger returns the global logger.
func getLogger() *slog.Logger {
	return globalLogger.Load()
}

// Infof logs informational messages about the general state of the profiler.
// This function is a wrapper around the structured slog-based logger,
// formatting the message as a string for backward compatibility with
// previous unstructured logging.
func Infof(msg string, keysAndValues ...any) {
	if getLogger().Enabled(context.Background(), slog.LevelInfo) {
		getLogger().Info(fmt.Sprintf(msg, keysAndValues...))
	}
}

// Info logs informational messages about the general state of the profiler.
// This is a wrapper around Infof for convenience.
func Info(msg string) {
	if getLogger().Enabled(context.Background(), slog.LevelInfo) {
		getLogger().Info(msg)
	}
}

// Errorf logs error messages about exceptional states of the profiler.
// This wrapper formats structured log data into a string message for
// backward compatibility with older unstructured logs.
func Errorf(msg string, keysAndValues ...any) {
	if getLogger().Enabled(context.Background(), slog.LevelError) {
		getLogger().Error(fmt.Sprintf(msg, keysAndValues...))
	}
}

// Error logs error messages about exceptional states of the profiler.
// This is a wrapper around Errorf for convenience.
func Error(msg error) {
	if getLogger().Enabled(context.Background(), slog.LevelError) {
		getLogger().Error(msg.Error())
	}
}

// Debugf logs detailed debugging information about internal profiler behavior.
// This wrapper converts structured log data into a string message for
// backward compatibility with older unstructured logs.
func Debugf(msg string, keysAndValues ...any) {
	if getLogger().Enabled(context.Background(), slog.LevelDebug) {
		getLogger().Debug(fmt.Sprintf(msg, keysAndValues...))
	}
}

// Debug logs detailed debugging information about internal profiler behavior.
// This is a wrapper around Debugf for convenience.
func Debug(msg string) {
	if getLogger().Enabled(context.Background(), slog.LevelDebug) {
		getLogger().Debug(msg)
	}
}

// Warnf logs warnings in the profiler — not errors, but likely more important
// than informational messages. This wrapper preserves backward compatibility
// by string-formatting structured log data.
func Warnf(msg string, keysAndValues ...any) {
	if getLogger().Enabled(context.Background(), slog.LevelWarn) {
		getLogger().Warn(fmt.Sprintf(msg, keysAndValues...))
	}
}

// Warn logs warnings in the profiler — not errors, but likely more important
// than informational messages. This is a wrapper around Warnf for convenience.
func Warn(msg string) {
	if getLogger().Enabled(context.Background(), slog.LevelWarn) {
		getLogger().Warn(msg)
	}
}
