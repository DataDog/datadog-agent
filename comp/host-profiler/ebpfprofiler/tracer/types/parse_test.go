// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package types

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTracers(t *testing.T) {
	testCases := []struct {
		name            string
		input           string
		expectedTracers []tracerType
	}{
		{name: "all", input: "all", expectedTracers: nil},
		{name: "all with trailing comma", input: "all,", expectedTracers: nil},
		{name: "all with native", input: "all,native", expectedTracers: nil},
		{name: "native only", input: "native", expectedTracers: []tracerType{}},
		{name: "native and php", input: "native,php", expectedTracers: []tracerType{PHPTracer}},
		{name: "native and python", input: "native,python", expectedTracers: []tracerType{PythonTracer}},
		{name: "native and two tracers", input: "native,php,python", expectedTracers: []tracerType{PHPTracer, PythonTracer}},
		{name: "dotnet and ruby", input: "dotnet,ruby", expectedTracers: []tracerType{DotnetTracer, RubyTracer}},
		{name: "go is ignored", input: "go", expectedTracers: []tracerType{}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			include, err := Parse(testCase.input)
			require.NoError(t, err)

			if testCase.expectedTracers == nil {
				for tracer := tracerType(0); tracer < maxTracers; tracer++ {
					require.Equal(t, availableOnArch(tracer), include.Has(tracer))
				}
				return
			}

			expected := strings.Split(testCase.input, ",")
			for tracer := tracerType(0); tracer < maxTracers; tracer++ {
				require.Equal(t, hasTracer(expected, tracer.String()) && availableOnArch(tracer), include.Has(tracer))
			}
		})
	}
}

func TestAllTracers(t *testing.T) {
	tracers := AllTracers()
	for tracer := tracerType(0); tracer < maxTracers; tracer++ {
		require.Equal(t, tracer != GoTracer, tracers.Has(tracer))
	}
}

func TestEnableDisable(t *testing.T) {
	var tracers IncludedTracers

	tracers.Enable(PythonTracer)
	tracers.Enable(Labels)
	require.True(t, tracers.Has(PythonTracer))
	require.True(t, tracers.Has(Labels))
	require.Equal(t, "python,labels", tracers.String())

	tracers.Disable(PythonTracer)
	require.False(t, tracers.Has(PythonTracer))
	require.Equal(t, "labels", tracers.String())
}

func availableOnArch(tracer tracerType) bool {
	if tracer == GoTracer {
		return false
	}
	if runtime.GOARCH == "arm64" {
		return tracer != DotnetTracer
	}
	return true
}

func hasTracer(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}
