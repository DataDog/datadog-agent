// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package log

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
	"time"
)

var _ LoggerInterface = (*SlogWrapper)(nil)

type SlogWrapper struct {
	logger          *slog.Logger
	extraStackDepth int
	levelVar        levelVar
	flush           func()
	close           func() error
}

type levelVar interface {
	slog.Leveler
	Set(slog.Level)
}

type fixedLevelVar struct {
	level slog.Level
}

func newFixedLevelVar(level slog.Level) *fixedLevelVar {
	return &fixedLevelVar{level: level}
}

func (f *fixedLevelVar) Level() slog.Level    { return f.level }
func (f *fixedLevelVar) Set(level slog.Level) { f.level = level }

// NewSlogWrapper creates a new SlogWrapper with the given logger and extra stack depth.
func NewSlogWrapper(logger *slog.Logger, extraStackDepth int, levelVar *slog.LevelVar, flush func(), close func() error) *SlogWrapper {
	return &SlogWrapper{
		logger:          logger,
		extraStackDepth: extraStackDepth,
		levelVar:        levelVar,
		flush:           flush,
		close:           close,
	}
}

// NewSlogWrapperFixedLevel creates a new SlogWrapper with the given logger and extra stack depth.
func NewSlogWrapperFixedLevel(logger *slog.Logger, extraStackDepth int, level LogLevel, flush func(), close func() error) *SlogWrapper {
	return &SlogWrapper{
		logger:          logger,
		extraStackDepth: extraStackDepth,
		levelVar:        newFixedLevelVar(slog.Level(level)),
		flush:           flush,
		close:           close,
	}
}

// Logger returns the inner logger
func (s *SlogWrapper) Logger() *slog.Logger {
	return s.logger
}

// LogLevel returns the current log level
func (s *SlogWrapper) LogLevel() LogLevel {
	return LogLevel(s.levelVar.Level())
}

// SetLevel sets the level of the inner logger
func (s *SlogWrapper) SetLevel(level LogLevel) {
	s.levelVar.Set(slog.Level(level))
}

// Logf logs a message with the given level and format
func (s *SlogWrapper) Logf(level LogLevel, extraStackDepth int, contexts []interface{}, format string, args ...interface{}) {
	sloglevel := slog.Level(level)
	if !s.logger.Enabled(context.Background(), sloglevel) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2+extraStackDepth, pcs[:])
	r := slog.NewRecord(time.Now(), sloglevel, fmt.Sprintf(format, args...), pcs[0])
	_ = s.logger.With(contexts...).Handler().Handle(context.Background(), r)
}

// LogfError logs a message with the given level and format, and returns an error if the message is not enabled
func (s *SlogWrapper) LogfError(level LogLevel, extraStackDepth int, contexts []interface{}, format string, args ...interface{}) error {
	sloglevel := slog.Level(level)
	err := formatErrorc(fmt.Sprintf(format, args...), contexts...)
	if !s.logger.Enabled(context.Background(), sloglevel) {
		return err
	}
	var pcs [1]uintptr
	runtime.Callers(2+extraStackDepth, pcs[:])
	r := slog.NewRecord(time.Now(), sloglevel, err.Error(), pcs[0])
	_ = s.logger.With(contexts...).Handler().Handle(context.Background(), r)
	return err
}

// Log logs a message with the given level and arguments
func (s *SlogWrapper) Log(level LogLevel, extraStackDepth int, contexts []interface{}, args ...interface{}) {
	sloglevel := slog.Level(level)
	if !s.logger.Enabled(context.Background(), sloglevel) {
		return
	}
	var pcs [1]uintptr
	runtime.Callers(2+extraStackDepth, pcs[:])
	r := slog.NewRecord(time.Now(), sloglevel, BuildLogEntry(args...), pcs[0])
	_ = s.logger.With(contexts...).Handler().Handle(context.Background(), r)
}

// LogError logs a message with the given level and arguments, and returns an error if the message is not enabled
func (s *SlogWrapper) LogError(level LogLevel, extraStackDepth int, contexts []interface{}, args ...interface{}) error {
	sloglevel := slog.Level(level)
	err := formatErrorc(BuildLogEntry(args...), contexts...)
	if !s.logger.Enabled(context.Background(), sloglevel) {
		return err
	}
	var pcs [1]uintptr
	runtime.Callers(2+extraStackDepth, pcs[:])
	r := slog.NewRecord(time.Now(), sloglevel, err.Error(), pcs[0])
	_ = s.logger.With(contexts...).Handler().Handle(context.Background(), r)
	return err
}

// Flush flushes the inner logger
func (s *SlogWrapper) Flush() {
	if s.flush != nil {
		s.flush()
	}
}

// Close closes the inner logger
func (s *SlogWrapper) Close() error {
	if s.close != nil {
		return s.close()
	}
	return nil
}

// Tracef formats message according to format specifier
// and writes to log with level = Trace.
func (s *SlogWrapper) Tracef(format string, params ...interface{}) {
	s.Logf(TraceLvl, 1, nil, format, params...)
}

// Debugf formats message according to format specifier
// and writes to log with level = Debug.
func (s *SlogWrapper) Debugf(format string, params ...interface{}) {
	s.Logf(DebugLvl, 1, nil, format, params...)
}

// Infof formats message according to format specifier
// and writes to log with level = Info.
func (s *SlogWrapper) Infof(format string, params ...interface{}) {
	s.Logf(InfoLvl, 1, nil, format, params...)
}

// Warnf formats message according to format specifier
// and writes to log with level = Warn.
func (s *SlogWrapper) Warnf(format string, params ...interface{}) error {
	return s.LogfError(WarnLvl, 1, nil, format, params...)
}

// Errorf formats message according to format specifier
// and writes to log with level = Error.
func (s *SlogWrapper) Errorf(format string, params ...interface{}) error {
	return s.LogfError(ErrorLvl, 1, nil, format, params...)
}

// Criticalf formats message according to format specifier
// and writes to log with level = Critical.
func (s *SlogWrapper) Criticalf(format string, params ...interface{}) error {
	return s.LogfError(CriticalLvl, 1, nil, format, params...)
}

// Trace formats message using the default formats for its operands
// and writes to log with level = Trace
func (s *SlogWrapper) Trace(v ...interface{}) {
	s.Log(TraceLvl, 1, nil, v...)
}

// Debug formats message using the default formats for its operands
// and writes to log with level = Debug
func (s *SlogWrapper) Debug(v ...interface{}) {
	s.Log(DebugLvl, 1, nil, v...)
}

// Info formats message using the default formats for its operands
// and writes to log with level = Info
func (s *SlogWrapper) Info(v ...interface{}) {
	s.Log(InfoLvl, 1, nil, v...)
}

// Warn formats message using the default formats for its operands
// and writes to log with level = Warn
func (s *SlogWrapper) Warn(v ...interface{}) error {
	return s.LogError(WarnLvl, 1, nil, v...)
}

// Error formats message using the default formats for its operands
// and writes to log with level = Error
func (s *SlogWrapper) Error(v ...interface{}) error {
	return s.LogError(ErrorLvl, 1, nil, v...)
}

// Critical formats message using the default formats for its operands
// and writes to log with level = Critical
func (s *SlogWrapper) Critical(v ...interface{}) error {
	return s.LogError(CriticalLvl, 1, nil, v...)
}
