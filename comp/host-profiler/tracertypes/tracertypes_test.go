// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracertypes

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr string
	}{
		{
			name:  "explicit tracers",
			input: "python, ruby",
			want:  "python,ruby",
		},
		{
			name:  "all includes every tracer",
			input: "all",
			want:  expectedAllTracersString(),
		},
		{
			name:  "native is ignored",
			input: "native,go",
			want:  "go",
		},
		{
			name:  "empty entries are ignored",
			input: "python,,go",
			want:  "python,go",
		},
		{
			name:    "unknown tracer",
			input:   "python,unknown",
			wantErr: "unknown tracer: unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracers, err := Parse(tt.input)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, tracers.String())
		})
	}
}

func TestAllTracers(t *testing.T) {
	require.Equal(t, expectedAllTracersStringWithoutArchFilter(), AllTracers().String())
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

func expectedAllTracersString() string {
	if runtime.GOARCH == "arm64" {
		return "perl,php,python,hotspot,ruby,v8,go,labels,beam"
	}

	return expectedAllTracersStringWithoutArchFilter()
}

func expectedAllTracersStringWithoutArchFilter() string {
	return "perl,php,python,hotspot,ruby,v8,dotnet,go,labels,beam"
}
