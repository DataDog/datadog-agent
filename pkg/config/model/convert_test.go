// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertToDefaultType(t *testing.T) {
	tests := []struct {
		name         string
		value        interface{}
		defaultValue interface{}
		want         interface{}
	}{
		{"nil default passes through", "foo", nil, "foo"},
		{"bool", "true", false, true},
		{"string from int", 42, "", "42"},
		{"int from float64", float64(16), 0, 16},
		{"int32 from float64", float64(16), int32(0), 16},
		{"int64 from float64", float64(65432), int64(0), int64(65432)},
		{"uint from float64", float64(1024), uint(0), uint(1024)},
		{"uint64 from float64", float64(1024), uint64(0), uint64(1024)},
		{"float64 from int", 16, float64(0), float64(16)},
		{"float32 from int", 16, float32(0), float64(16)},
		{"duration from string", "30s", time.Duration(0), 30 * time.Second},
		{"string slice from []interface{}", []interface{}{"a", "b"}, []string{}, []string{"a", "b"}},
		{"float64 slice from []interface{}", []interface{}{float64(1), float64(2.5)}, []float64{}, []float64{1, 2.5}},
		{"map default passes through", map[string]interface{}{"k": "v"}, map[string]string{}, map[string]interface{}{"k": "v"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertToDefaultType(tc.value, tc.defaultValue)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
