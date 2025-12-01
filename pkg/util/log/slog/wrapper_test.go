// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package slog

import (
	"context"
	"log/slog"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log/types"
)

// mockHandler is a simple slog.Handler that records log messages
type mockHandler struct {
	records   []slog.Record
	enabled   bool
	lastLevel slog.Level
}

func newMockHandler() *mockHandler {
	return &mockHandler{
		records: make([]slog.Record, 0),
		enabled: true,
	}
}

func (m *mockHandler) Enabled(_ context.Context, level slog.Level) bool {
	m.lastLevel = level
	return m.enabled
}

func (m *mockHandler) Handle(_ context.Context, record slog.Record) error {
	m.records = append(m.records, record)
	return nil
}

func (m *mockHandler) WithAttrs(_ []slog.Attr) slog.Handler {
	return m
}

func (m *mockHandler) WithGroup(_ string) slog.Handler {
	return m
}

func (m *mockHandler) lastMessage() string {
	if len(m.records) == 0 {
		return ""
	}
	return m.records[len(m.records)-1].Message
}

func (m *mockHandler) lastRecord() slog.Record {
	if len(m.records) == 0 {
		return slog.Record{}
	}
	return m.records[len(m.records)-1]
}

func (m *mockHandler) reset() {
	m.records = make([]slog.Record, 0)
}

func TestNewWrapperWithCloseAndFlush(t *testing.T) {
	handler := newMockHandler()
	flushCalled := false
	closeCalled := false

	flushFunc := func() { flushCalled = true }
	closeFunc := func() { closeCalled = true }

	wrapper := NewWrapperWithCloseAndFlush(handler, flushFunc, closeFunc)

	wrapper.Flush()
	assert.True(t, flushCalled)

	wrapper.Close()
	assert.True(t, closeCalled)
}

func TestWrapperBasicLogLevels(t *testing.T) {
	tests := []struct {
		name              string
		logFunc           func(types.LoggerInterface, ...interface{})
		logfFunc          func(types.LoggerInterface, string, ...interface{})
		expectedLevel     types.LogLevel
		argsInput         []interface{}
		expectedArgsMsg   string
		formatMsg         string
		formatArgs        []interface{}
		expectedFormatMsg string
	}{
		{
			name:              "Trace",
			logFunc:           types.LoggerInterface.Trace,
			logfFunc:          types.LoggerInterface.Tracef,
			expectedLevel:     types.TraceLvl,
			argsInput:         []interface{}{"test ", "message"},
			expectedArgsMsg:   "test message",
			formatMsg:         "test %s %d",
			formatArgs:        []interface{}{"message", 42},
			expectedFormatMsg: "test message 42",
		},
		{
			name:              "Debug",
			logFunc:           types.LoggerInterface.Debug,
			logfFunc:          types.LoggerInterface.Debugf,
			expectedLevel:     types.DebugLvl,
			argsInput:         []interface{}{"debug ", "message"},
			expectedArgsMsg:   "debug message",
			formatMsg:         "debug %s %d",
			formatArgs:        []interface{}{"message", 123},
			expectedFormatMsg: "debug message 123",
		},
		{
			name:              "Info",
			logFunc:           types.LoggerInterface.Info,
			logfFunc:          types.LoggerInterface.Infof,
			expectedLevel:     types.InfoLvl,
			argsInput:         []interface{}{"info ", "message"},
			expectedArgsMsg:   "info message",
			formatMsg:         "info %s",
			formatArgs:        []interface{}{"message"},
			expectedFormatMsg: "info message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newMockHandler()
			wrapper := NewWrapper(handler)

			// Test args version
			tt.logFunc(wrapper, tt.argsInput...)
			assert.Equal(t, tt.expectedArgsMsg, handler.lastMessage())
			require.Equal(t, 1, len(handler.records), handler.records)
			assert.Equal(t, tt.expectedLevel, types.FromSlogLevel(handler.lastRecord().Level))

			// Reset for format test
			handler.reset()

			// Test format version
			tt.logfFunc(wrapper, tt.formatMsg, tt.formatArgs...)
			assert.Equal(t, tt.expectedFormatMsg, handler.lastMessage())
			require.Equal(t, 1, len(handler.records), handler.records)
			assert.Equal(t, tt.expectedLevel, types.FromSlogLevel(handler.lastRecord().Level))
		})
	}
}

func TestWrapperErrorLogLevels(t *testing.T) {
	tests := []struct {
		name              string
		logFunc           func(types.LoggerInterface, ...interface{}) error
		logfFunc          func(types.LoggerInterface, string, ...interface{}) error
		expectedLevel     types.LogLevel
		argsInput         []interface{}
		expectedArgsMsg   string
		formatMsg         string
		formatArgs        []interface{}
		expectedFormatMsg string
	}{
		{
			name:              "Warn",
			logFunc:           types.LoggerInterface.Warn,
			logfFunc:          types.LoggerInterface.Warnf,
			expectedLevel:     types.WarnLvl,
			argsInput:         []interface{}{"warn ", "message"},
			expectedArgsMsg:   "warn message",
			formatMsg:         "warn %s",
			formatArgs:        []interface{}{"message"},
			expectedFormatMsg: "warn message",
		},
		{
			name:              "Error",
			logFunc:           types.LoggerInterface.Error,
			logfFunc:          types.LoggerInterface.Errorf,
			expectedLevel:     types.ErrorLvl,
			argsInput:         []interface{}{"error ", "message"},
			expectedArgsMsg:   "error message",
			formatMsg:         "error %d",
			formatArgs:        []interface{}{404},
			expectedFormatMsg: "error 404",
		},
		{
			name:              "Critical",
			logFunc:           types.LoggerInterface.Critical,
			logfFunc:          types.LoggerInterface.Criticalf,
			expectedLevel:     types.CriticalLvl,
			argsInput:         []interface{}{"critical ", "message"},
			expectedArgsMsg:   "critical message",
			formatMsg:         "critical %s",
			formatArgs:        []interface{}{"failure"},
			expectedFormatMsg: "critical failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newMockHandler()
			wrapper := NewWrapper(handler)

			// Test args version
			err := tt.logFunc(wrapper, tt.argsInput...)
			require.Equal(t, 1, len(handler.records), handler.records)
			assert.Equal(t, tt.expectedArgsMsg, handler.lastMessage())
			assert.Equal(t, tt.expectedLevel, types.FromSlogLevel(handler.lastRecord().Level))
			assert.Error(t, err)
			assert.Equal(t, tt.expectedArgsMsg, err.Error())

			// Reset for format test
			handler.reset()

			// Test format version
			err = tt.logfFunc(wrapper, tt.formatMsg, tt.formatArgs...)
			require.Equal(t, 1, len(handler.records), handler.records)
			assert.Equal(t, tt.expectedFormatMsg, handler.lastMessage())
			assert.Equal(t, tt.expectedLevel, types.FromSlogLevel(handler.lastRecord().Level))
			assert.Error(t, err)
			assert.Equal(t, tt.expectedFormatMsg, err.Error())
		})
	}
}

func TestWrapperSetAdditionalStackDepth(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Helper function that logs a message
	logHelper := func() {
		wrapper.Info("test message")
	}

	// Log without extra stack depth - should capture the anonymous function (func1) as the caller
	logHelper()
	require.Equal(t, 1, len(handler.records), handler.records)
	record1 := handler.lastRecord()
	frame1, _ := runtime.CallersFrames([]uintptr{record1.PC}).Next()
	assert.Contains(t, frame1.Function, "TestWrapperSetAdditionalStackDepth.func1")

	// Reset records
	handler.reset()

	// Set additional stack depth to skip one more frame (the helper function)
	err := wrapper.SetAdditionalStackDepth(1)
	require.NoError(t, err)

	// Log again with extra stack depth - should now capture TestWrapperSetAdditionalStackDepth as the caller
	logHelper()
	require.Equal(t, 1, len(handler.records), handler.records)
	record2 := handler.lastRecord()
	frame2, _ := runtime.CallersFrames([]uintptr{record2.PC}).Next()
	assert.True(t, strings.HasSuffix(frame2.Function, "TestWrapperSetAdditionalStackDepth"), frame2.Function)
}

func TestWrapperSetContext(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Test setting context
	context := []interface{}{"key1", "value1", "key2", 42}
	wrapper.SetContext(context)

	// Test that context is added to log record
	wrapper.Info("test")
	record := handler.lastRecord()
	attrs := getAttrs(record)
	require.Equal(t, 2, len(attrs))
	assert.Equal(t, "key1", attrs[0].Key)
	assert.Equal(t, "value1", attrs[0].Value.String())
	assert.Equal(t, "key2", attrs[1].Key)
	assert.Equal(t, int64(42), attrs[1].Value.Int64())

}

func getAttrs(record slog.Record) []slog.Attr {
	var attrs []slog.Attr
	record.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	return attrs
}

func TestWrapperSetContextNil(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Set context first
	context := []interface{}{"key", "value"}
	wrapper.SetContext(context)

	// Log and verify context is present
	wrapper.Info("test with context")
	attrs1 := getAttrs(handler.lastRecord())
	require.Equal(t, 1, len(attrs1))
	assert.Equal(t, "key", attrs1[0].Key)
	assert.Equal(t, "value", attrs1[0].Value.String())

	// Clear context
	handler.reset()
	wrapper.SetContext(nil)

	// Log and verify no context attributes are present
	wrapper.Info("test without context")
	record2 := handler.lastRecord()
	attrs2 := getAttrs(record2)
	assert.Empty(t, attrs2)
}

func TestWrapperSetContextSkipsNonStringKeys(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Context with non-string keys should be skipped
	context := []interface{}{123, "value1", "key2", "value2"}
	wrapper.SetContext(context)

	wrapper.Info("test with context")
	record := handler.lastRecord()
	attrs := getAttrs(record)
	require.Equal(t, 1, len(attrs))
	assert.Equal(t, "key2", attrs[0].Key)
	assert.Equal(t, "value2", attrs[0].Value.String())
}

func TestRenderArgs(t *testing.T) {
	result := renderArgs("hello", " ", "world")
	assert.Equal(t, "hello world", result)
}

func TestRenderFormat(t *testing.T) {
	result := renderFormat("hello %s %d", "world", 42)
	assert.Equal(t, "hello world 42", result)
}

// Test that complex types are properly formatted
func TestWrapperComplexTypes(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	type testStruct struct {
		Field1 string
		Field2 int
	}

	testObj := testStruct{Field1: "test", Field2: 42}

	wrapper.Info("struct: ", testObj)
	require.Equal(t, 1, len(handler.records), handler.records)
	// The exact format may vary, but it should contain the struct representation
	assert.Contains(t, handler.lastMessage(), "struct")
}

// Test multiple rapid log calls
func TestWrapperMultipleCalls(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	for i := 0; i < 100; i++ {
		wrapper.Info("message ", i)
	}

	assert.Equal(t, 100, len(handler.records))
	assert.Equal(t, "message 99", handler.lastMessage())
}

// Test empty messages
func TestWrapperEmptyMessage(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	wrapper.Info()
	assert.Equal(t, 1, len(handler.records))
	assert.Equal(t, "", strings.TrimSpace(handler.lastMessage()))
}

func TestWrapperFormatSpecifiers(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	wrapper.Infof("int: %d, string: %s, float: %.2f", 42, "test", 3.14159)
	assert.Equal(t, "int: 42, string: test, float: 3.14", handler.lastMessage())
}

// TestTracefDoesNotBuildMessageWhenDisabled verifies that Tracef doesn't build
// the message when the trace level is disabled, avoiding unnecessary formatting work
func TestTracefDoesNotBuildMessageWhenDisabled(t *testing.T) {
	handler := newMockHandler()
	handler.enabled = false
	wrapper := NewWrapper(handler)

	// Test that the fmt.Sprintf formatting is not called when level is disabled
	// We do this by using handleLazy directly with a custom stringer
	formatCalled := false
	msg := &trackedStringer{
		StringFunc: func() string {
			formatCalled = true
			return "formatted message"
		},
	}

	// Call handleLazy directly with our tracked stringer
	wrapper.Tracef("%s", msg)

	// Verify String() was NOT called because the level was disabled
	require.False(t, formatCalled, "String() should not be called when level is disabled")
	require.Equal(t, 0, len(handler.records), "no record should be logged when level is disabled")

	// Now test with level enabled to verify String() IS called
	handler.enabled = true
	handler.reset()

	formatCalled = false
	msg2 := &trackedStringer{
		StringFunc: func() string {
			formatCalled = true
			return "formatted message"
		},
	}

	wrapper.Tracef("%s", msg2)

	// Verify String() WAS called because the level was enabled
	assert.True(t, formatCalled, "String() should be called when level is enabled")
	assert.Equal(t, 1, len(handler.records), "record should be logged when level is enabled")
	assert.Equal(t, "formatted message", handler.lastMessage())
}

// Helper type for testing lazy evaluation
type trackedStringer struct {
	StringFunc func() string
}

func (t *trackedStringer) String() string {
	return t.StringFunc()
}

func TestContextValuesNotRendered(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Create a context value with a tracked stringer to detect if it's evaluated
	stringerCalled := false
	expensiveValue := &trackedStringer{
		StringFunc: func() string {
			stringerCalled = true
			return "expensive computation result"
		},
	}

	// Set context with the expensive stringer value
	wrapper.SetContext([]interface{}{"expensive_key", expensiveValue})
	assert.False(t, stringerCalled, "String() should not be called during SetContext")

	wrapper.Trace("test message")
	assert.False(t, stringerCalled, "String() should not be called by the wrapper itself")
}

func TestLoggingToClosedWrapperDoesNotCallHandler(t *testing.T) {
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	// Close the wrapper
	wrapper.Close()

	// Attempt to log at various levels after closing
	wrapper.Trace("trace message")
	wrapper.Tracef("trace message %s", "formatted")
	wrapper.Debug("debug message")
	wrapper.Debugf("debug message %s", "formatted")
	wrapper.Info("info message")
	wrapper.Infof("info message %s", "formatted")
	wrapper.Warn("warn message")
	wrapper.Warnf("warn message %s", "formatted")
	wrapper.Error("error message")
	wrapper.Errorf("error message %s", "formatted")
	wrapper.Critical("critical message")
	wrapper.Criticalf("critical message %s", "formatted")

	// Verify that handler was never called (no records logged)
	assert.Empty(t, handler.records, "handler should not be called after wrapper is closed")
}

func TestRecordTimeZone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows uses syscalls to get the timezone, so we can't test it")
	}

	t.Setenv("TZ", "CET")
	handler := newMockHandler()
	wrapper := NewWrapper(handler)

	wrapper.Info("test message")
	record := handler.lastRecord()
	assert.Equal(t, "CET", record.Time.Location().String())
}
