// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package types

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func makeTaskWithTimeout(v interface{}) *Task {
	t := &Task{}
	t.Data.Attributes = &Attributes{
		Inputs: map[string]interface{}{"timeout": v},
	}
	return t
}

func int32ptr(v int32) *int32 { return &v }

func TestTimeoutSeconds(t *testing.T) {
	tests := []struct {
		name string
		task *Task
		want *int32
	}{
		{
			name: "nil attributes",
			task: &Task{},
			want: nil,
		},
		{
			name: "missing key",
			task: func() *Task {
				task := &Task{}
				task.Data.Attributes = &Attributes{Inputs: map[string]interface{}{}}
				return task
			}(),
			want: nil,
		},
		// Valid float64 (normal JSON numbers)
		{
			name: "float64 valid",
			task: makeTaskWithTimeout(float64(30)),
			want: int32ptr(30),
		},
		// float64 overflow (> MaxInt32) — must fall back to nil
		{
			name: "float64 overflow",
			task: makeTaskWithTimeout(float64(math.MaxInt32) + 1),
			want: nil,
		},
		// float64 negative — must fall back to nil
		{
			name: "float64 negative",
			task: makeTaskWithTimeout(float64(-1)),
			want: nil,
		},
		// float64 zero — must fall back to nil
		{
			name: "float64 zero",
			task: makeTaskWithTimeout(float64(0)),
			want: nil,
		},
		// Non-integer float — must fall back to nil
		{
			name: "float64 non-integer",
			task: makeTaskWithTimeout(float64(1.5)),
			want: nil,
		},
		// int32 valid
		{
			name: "int32 valid",
			task: makeTaskWithTimeout(int32(60)),
			want: int32ptr(60),
		},
		// int32 negative
		{
			name: "int32 negative",
			task: makeTaskWithTimeout(int32(-5)),
			want: nil,
		},
		// int64 within range
		{
			name: "int64 valid",
			task: makeTaskWithTimeout(int64(120)),
			want: int32ptr(120),
		},
		// int64 overflow
		{
			name: "int64 overflow",
			task: makeTaskWithTimeout(int64(math.MaxInt32) + 1),
			want: nil,
		},
		// int within range
		{
			name: "int valid",
			task: makeTaskWithTimeout(int(90)),
			want: int32ptr(90),
		},
		// string value — must fall back to nil
		{
			name: "string value",
			task: makeTaskWithTimeout("30"),
			want: nil,
		},
		// bool value — must fall back to nil
		{
			name: "bool value",
			task: makeTaskWithTimeout(true),
			want: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.task.TimeoutSeconds()
			if tc.want == nil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, *tc.want, *got)
			}
		})
	}
}
