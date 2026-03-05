// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet

package kubelet

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuantity_DecimalSI(t *testing.T) {
	tests := []struct {
		input      string
		wantValue  int64
		wantMilli  int64
		wantFloat  float64
		wantString string
	}{
		{"0", 0, 0, 0, "0"},
		{"1", 1, 1000, 1.0, "1"},
		{"100", 100, 100000, 100.0, "100"},
		{"100m", 0, 100, 0.1, "100m"},
		{"500m", 0, 500, 0.5, "500m"},
		{"1000m", 1, 1000, 1.0, "1000m"},
		{"1500m", 1, 1500, 1.5, "1500m"},
		{"250m", 0, 250, 0.25, "250m"},
		{"1k", 1000, 1000000, 1000.0, "1k"},
		{"1M", 1000000, 1000000000, 1000000.0, "1M"},
		{"1G", 1000000000, 1000000000000, 1000000000.0, "1G"},
		{"1T", 1000000000000, 1000000000000000, 1000000000000.0, "1T"},
		{"100n", 0, 0, 100e-9, "100n"},
		{"1u", 0, 0, 1e-6, "1u"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			assert.Equal(t, tt.wantValue, q.Value(), "Value()")
			assert.Equal(t, tt.wantMilli, q.MilliValue(), "MilliValue()")
			assert.InDelta(t, tt.wantFloat, q.AsApproximateFloat64(), 1e-9, "AsApproximateFloat64()")
			assert.Equal(t, tt.wantString, q.String(), "String()")
		})
	}
}

func TestQuantity_BinarySI(t *testing.T) {
	tests := []struct {
		input     string
		wantValue int64
		wantFloat float64
	}{
		{"1Ki", 1024, 1024.0},
		{"1Mi", 1048576, 1048576.0},
		{"128Mi", 134217728, 134217728.0},
		{"1Gi", 1073741824, 1073741824.0},
		{"2Gi", 2147483648, 2147483648.0},
		{"1Ti", 1099511627776, 1099511627776.0},
		{"1Pi", 1125899906842624, 1125899906842624.0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			assert.Equal(t, tt.wantValue, q.Value(), "Value()")
			assert.InDelta(t, tt.wantFloat, q.AsApproximateFloat64(), 1.0, "AsApproximateFloat64()")
		})
	}
}

func TestQuantity_Decimals(t *testing.T) {
	tests := []struct {
		input     string
		wantValue int64
		wantMilli int64
		wantFloat float64
	}{
		{"0.5", 0, 500, 0.5},
		{"1.5", 1, 1500, 1.5},
		{"0.1", 0, 100, 0.1},
		{"0.001", 0, 1, 0.001},
		{"2.5", 2, 2500, 2.5},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			assert.Equal(t, tt.wantValue, q.Value(), "Value()")
			assert.Equal(t, tt.wantMilli, q.MilliValue(), "MilliValue()")
			assert.InDelta(t, tt.wantFloat, q.AsApproximateFloat64(), 1e-9, "AsApproximateFloat64()")
		})
	}
}

func TestQuantity_ExponentNotation(t *testing.T) {
	tests := []struct {
		input     string
		wantValue int64
		wantFloat float64
	}{
		{"1e3", 1000, 1000.0},
		{"1E3", 1000, 1000.0},
		{"5e2", 500, 500.0},
		{"1e0", 1, 1.0},
		{"1e-3", 0, 0.001},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			assert.Equal(t, tt.wantValue, q.Value(), "Value()")
			assert.InDelta(t, tt.wantFloat, q.AsApproximateFloat64(), 1e-9, "AsApproximateFloat64()")
		})
	}
}

func TestQuantity_JSONUnmarshal(t *testing.T) {
	type testStruct struct {
		CPU    Quantity `json:"cpu"`
		Memory Quantity `json:"memory"`
	}

	input := `{"cpu": "100m", "memory": "128Mi"}`
	var s testStruct
	err := json.Unmarshal([]byte(input), &s)
	require.NoError(t, err)

	assert.Equal(t, int64(0), s.CPU.Value())
	assert.Equal(t, int64(100), s.CPU.MilliValue())
	assert.InDelta(t, 0.1, s.CPU.AsApproximateFloat64(), 1e-9)

	assert.Equal(t, int64(128*1024*1024), s.Memory.Value())
}

func TestQuantity_JSONMarshal(t *testing.T) {
	q := MustParseQuantity("100m")
	data, err := json.Marshal(q)
	require.NoError(t, err)
	assert.Equal(t, `"100m"`, string(data))
}

func TestQuantity_JSONRoundTrip(t *testing.T) {
	type testStruct struct {
		Value Quantity `json:"value"`
	}

	original := testStruct{Value: MustParseQuantity("256Mi")}
	data, err := json.Marshal(original)
	require.NoError(t, err)

	var decoded testStruct
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, original.Value.String(), decoded.Value.String())
	assert.Equal(t, original.Value.Value(), decoded.Value.Value())
}

func TestQuantity_ZeroValue(t *testing.T) {
	var q Quantity
	assert.Equal(t, int64(0), q.Value())
	assert.Equal(t, int64(0), q.MilliValue())
	assert.Equal(t, 0.0, q.AsApproximateFloat64())
	assert.Equal(t, "", q.String())
}

func TestQuantity_NullJSON(t *testing.T) {
	var q Quantity
	err := q.UnmarshalJSON([]byte("null"))
	require.NoError(t, err)
	assert.Equal(t, int64(0), q.Value())
}

func TestQuantity_InvalidInput(t *testing.T) {
	invalidInputs := []string{
		"",
		"abc",
		"m",
	}

	for _, input := range invalidInputs {
		t.Run(input, func(t *testing.T) {
			_, err := parseQuantity(input)
			assert.Error(t, err)
		})
	}
}

func TestQuantity_WholeCoreCheck(t *testing.T) {
	// Used by the codebase to check if CPU request is a whole core
	q1 := MustParseQuantity("1")
	assert.Equal(t, int64(0), q1.MilliValue()%1000, "1 is a whole core")

	q2 := MustParseQuantity("100m")
	assert.NotEqual(t, int64(0), q2.MilliValue()%1000, "100m is not a whole core")

	q3 := MustParseQuantity("2")
	assert.Equal(t, int64(0), q3.MilliValue()%1000, "2 is whole cores")

	q4 := MustParseQuantity("1500m")
	assert.NotEqual(t, int64(0), q4.MilliValue()%1000, "1500m is not a whole core")
}

func TestQuantity_CPUFormatting(t *testing.T) {
	// Simulates FormatCPURequests: AsApproximateFloat64() * 100
	tests := []struct {
		input    string
		wantPct  float64
	}{
		{"100m", 10.0},    // 0.1 core = 10%
		{"250m", 25.0},    // 0.25 core = 25%
		{"500m", 50.0},    // 0.5 core = 50%
		{"1", 100.0},      // 1 core = 100%
		{"1500m", 150.0},  // 1.5 cores = 150%
		{"2", 200.0},      // 2 cores = 200%
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			pct := q.AsApproximateFloat64() * 100
			assert.InDelta(t, tt.wantPct, pct, 0.01)
		})
	}
}

func TestQuantity_MemoryFormatting(t *testing.T) {
	// Simulates FormatMemoryRequests: uint64(Value())
	tests := []struct {
		input     string
		wantBytes uint64
	}{
		{"128Mi", 128 * 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"256Mi", 256 * 1024 * 1024},
		{"512Mi", 512 * 1024 * 1024},
		{"1000000", 1000000},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			q := MustParseQuantity(tt.input)
			assert.Equal(t, tt.wantBytes, uint64(q.Value()))
		})
	}
}

func TestQuantity_NegativeValues(t *testing.T) {
	q := MustParseQuantity("-100m")
	assert.Equal(t, int64(-100), q.MilliValue())
	assert.InDelta(t, -0.1, q.AsApproximateFloat64(), 1e-9)
}

func TestQuantity_LargeEiSuffix(t *testing.T) {
	q := MustParseQuantity("1Ei")
	expected := int64(1) << 60
	assert.Equal(t, expected, q.Value())
	assert.InDelta(t, float64(expected), q.AsApproximateFloat64(), math.Abs(float64(expected)*1e-10))
}

func TestMustParseQuantity_Panics(t *testing.T) {
	assert.Panics(t, func() {
		MustParseQuantity("")
	})
}
