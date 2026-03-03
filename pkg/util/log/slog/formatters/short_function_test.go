// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package formatters

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShortFunction(t *testing.T) {
	tests := []struct {
		name     string
		function string
		expected string
	}{
		{
			name:     "simple function",
			function: "main.main",
			expected: "main",
		},
		{
			name:     "package function",
			function: "github.com/DataDog/datadog-agent/pkg/util/log.Info",
			expected: "Info",
		},
		{
			name:     "nested package",
			function: "github.com/DataDog/datadog-agent/pkg/util/log/slog.TestShortFunction",
			expected: "TestShortFunction",
		},
		{
			name:     "method on struct",
			function: "github.com/DataDog/datadog-agent/pkg/util/log.(*Logger).Info",
			expected: "Info",
		},
		{
			name:     "closure",
			function: "github.com/DataDog/datadog-agent/pkg/util/log.TestFunc.func1",
			expected: "func1",
		},
		{
			name:     "no dot",
			function: "main",
			expected: "main",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			frame := runtime.Frame{
				Function: tt.function,
			}
			result := ShortFunction(frame)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestShortFunctionEmptyFrame(t *testing.T) {
	frame := runtime.Frame{
		Function: "",
	}
	result := ShortFunction(frame)
	// Should handle empty function name without panicking
	assert.Equal(t, "", result)
}

func TestShortFunctionRealFrame(t *testing.T) {
	// Get a real frame from the current call
	var pc [1]uintptr
	runtime.Callers(1, pc[:])
	frames := runtime.CallersFrames(pc[:])
	frame, _ := frames.Next()

	result := ShortFunction(frame)

	// Should extract the function name
	assert.NotEmpty(t, result)
	assert.NotContains(t, result, "/")
	assert.Equal(t, "TestShortFunctionRealFrame", result)
}

func helperFunction() runtime.Frame {
	var pc [1]uintptr
	runtime.Callers(1, pc[:])
	frames := runtime.CallersFrames(pc[:])
	frame, _ := frames.Next()
	return frame
}

func TestShortFunctionFromHelper(t *testing.T) {
	frame := helperFunction()
	result := ShortFunction(frame)

	assert.Equal(t, "helperFunction", result)
}
