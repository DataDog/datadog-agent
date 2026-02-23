// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logging provides a logging adapter that bridges to the original source code logging system
package logging

import (
	"context"
	"fmt"
	"strings"
	"time"

	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// contextKey is a private type for context keys
type contextKey int

// loggerContextKey is the key used to store the logger in the context.
const loggerContextKey contextKey = iota

// Field represents a structured logging field
type Field struct {
	Key   string
	Value interface{}
}

// Logger interface that mimics original logger behavior
type Logger interface {
	Debug(msg string, fields ...Field)
	Debugf(format string, args ...interface{})
	Info(msg string, fields ...Field)
	Infof(format string, args ...interface{})
	Warn(msg string, fields ...Field)
	Warnf(format string, args ...interface{})
	Error(msg string, fields ...Field)
	Errorf(format string, args ...interface{})
	With(fields ...Field) Logger
}

// loggerAdapter adapts datadog-agent logging to the original logging interface
type loggerAdapter struct {
	contextFields []Field
}

// FromContext returns the logger from ctx, or a new logger if none exists.
func FromContext(ctx context.Context) Logger {
	if logger, ok := ctx.Value(loggerContextKey).(Logger); ok && logger != nil {
		return logger
	}

	return &loggerAdapter{
		contextFields: []Field{},
	}
}

// ContextWithLogger returns a new context containing logger.
func ContextWithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, loggerContextKey, logger)
}

// String creates a string field
func String(key, value string) Field {
	return Field{Key: key, Value: value}
}

// Strings creates a string slice field
func Strings(key string, values []string) Field {
	return Field{Key: key, Value: values}
}

// Int creates an int field
func Int(key string, value int) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates an int32 field
func Int32(key string, value int32) Field {
	return Field{Key: key, Value: value}
}

// Int64 creates an int64 field
func Int64(key string, value int64) Field {
	return Field{Key: key, Value: value}
}

// Bool creates a bool field
func Bool(key string, value bool) Field {
	return Field{Key: key, Value: value}
}

// ErrorField creates an error field
func ErrorField(err error) Field {
	return Field{Key: "error", Value: err}
}

// Any creates a field with any value
func Any(key string, value interface{}) Field {
	return Field{Key: key, Value: value}
}

func Duration(key string, value time.Duration) Field {
	return Field{Key: key, Value: value}
}

// formatMessage combines message with fields into a formatted string
func (l *loggerAdapter) formatMessage(msg string, fields ...Field) string {
	allFields := append(l.contextFields, fields...)
	if len(allFields) == 0 {
		return msg
	}

	var parts []string
	var err error
	hasError := false

	for _, field := range allFields {
		if field.Key == "error" {
			if e, ok := field.Value.(error); ok {
				err = e
				hasError = true
				continue
			}
		}
		parts = append(parts, fmt.Sprintf("%s=%v", field.Key, field.Value))
	}

	if hasError && err != nil {
		if len(parts) > 0 {
			return fmt.Sprintf("%s: %v (%s)", msg, err, strings.Join(parts, " "))
		}
		return fmt.Sprintf("%s: %v", msg, err)
	}

	if len(parts) > 0 {
		return fmt.Sprintf("%s (%s)", msg, strings.Join(parts, " "))
	}
	return msg
}

// Debug logs at debug level
func (l *loggerAdapter) Debug(msg string, fields ...Field) {
	ddlog.Debug(l.formatMessage(msg, fields...))
}

// Debugf logs at debug level with format
func (l *loggerAdapter) Debugf(format string, args ...interface{}) {
	ddlog.Debugf(format, args...)
}

// Info logs at info level
func (l *loggerAdapter) Info(msg string, fields ...Field) {
	ddlog.Info(l.formatMessage(msg, fields...))
}

// Infof logs at info level with format
func (l *loggerAdapter) Infof(format string, args ...interface{}) {
	ddlog.Infof(format, args...)
}

// Warn logs at warn level
func (l *loggerAdapter) Warn(msg string, fields ...Field) {
	ddlog.Warn(l.formatMessage(msg, fields...))
}

// Warnf logs at warn level with format
func (l *loggerAdapter) Warnf(format string, args ...interface{}) {
	ddlog.Warnf(format, args...)
}

// Error logs at error level
func (l *loggerAdapter) Error(msg string, fields ...Field) {
	ddlog.Error(l.formatMessage(msg, fields...))
}

// Errorf logs at error level with format
func (l *loggerAdapter) Errorf(format string, args ...interface{}) {
	ddlog.Errorf(format, args...)
}

// With returns a new logger with additional context fields
func (l *loggerAdapter) With(fields ...Field) Logger {
	newFields := make([]Field, len(l.contextFields)+len(fields))
	copy(newFields, l.contextFields)
	copy(newFields[len(l.contextFields):], fields)

	return &loggerAdapter{
		contextFields: newFields,
	}
}

// Debug logs at debug level
func Debug(msg string, fields ...Field) {
	defaultLogger.Debug(msg, fields...)
}

// Debugf logs at debug level with format
func Debugf(format string, args ...interface{}) {
	ddlog.Debugf(format, args...)
}

// Info logs at info level
func Info(msg string, fields ...Field) {
	defaultLogger.Info(msg, fields...)
}

// Infof logs at info level with format
func Infof(format string, args ...interface{}) {
	ddlog.Infof(format, args...)
}

// Warn logs at warn level
func Warn(msg string, fields ...Field) {
	defaultLogger.Warn(msg, fields...)
}

// Warnf logs at warn level with format
func Warnf(format string, args ...interface{}) {
	ddlog.Warnf(format, args...)
}

// Error logs at error level
func Error(msg string, fields ...Field) {
	defaultLogger.Error(msg, fields...)
}

// Errorf logs at error level with format
func Errorf(format string, args ...interface{}) {
	ddlog.Errorf(format, args...)
}

// Critical logs at critical level (maps to datadog-agent Critical)
func Critical(msg string, fields ...Field) {
	ddlog.Critical(defaultLogger.formatMessage(msg, fields...))
}

// Criticalf logs at critical level with format
func Criticalf(format string, args ...interface{}) {
	ddlog.Criticalf(format, args...)
}

// Fatal logs at fatal level then exits (maps to Critical + exit)
func Fatal(msg string, fields ...Field) {
	ddlog.Critical(defaultLogger.formatMessage(msg, fields...))
	// Note: datadog-agent's Critical doesn't exit, so this behavior differs
	// The sync process or usage sites may need to handle this
}

// Fatalf logs at fatal level with format then exits
func Fatalf(format string, args ...interface{}) {
	ddlog.Criticalf(format, args...)
	// Note: datadog-agent's Criticalf doesn't exit, so this behavior differs
}

// Panic logs at panic level then panics
func Panic(msg string, fields ...Field) {
	message := defaultLogger.formatMessage(msg, fields...)
	ddlog.Critical(message)
	panic(message)
}

// Panicf logs at panic level with format then panics
func Panicf(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	ddlog.Critical(message)
	panic(message)
}

var defaultLogger = &loggerAdapter{}
