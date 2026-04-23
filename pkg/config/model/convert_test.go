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
		coerceMaps   bool
		want         interface{}
	}{
		{"nil default passes through", "foo", nil, false, "foo"},
		{"bool", "true", false, false, true},
		{"string from int", 42, "", false, "42"},
		{"int from float64", float64(16), 0, false, 16},
		{"int32 from float64", float64(16), int32(0), false, 16},
		{"int64 from float64", float64(65432), int64(0), false, int64(65432)},
		{"uint from float64", float64(1024), uint(0), false, uint(1024)},
		{"uint64 from float64", float64(1024), uint64(0), false, uint64(1024)},
		{"float64 from int", 16, float64(0), false, float64(16)},
		{"float32 from int", 16, float32(0), false, float64(16)},
		{"duration from string", "30s", time.Duration(0), false, 30 * time.Second},
		{"string slice from []interface{}", []interface{}{"a", "b"}, []string{}, false, []string{"a", "b"}},
		{"float64 slice from []interface{}", []interface{}{float64(1), float64(2.5)}, []float64{}, false, []float64{1, 2.5}},
		{"map pass-through when coerceMaps=false", map[string]interface{}{"k": "v"}, map[string]string{}, false, map[string]interface{}{"k": "v"}},
		{"map[string]string reshape when coerceMaps=true", map[string]interface{}{"k": "v"}, map[string]string{}, true, map[string]string{"k": "v"}},
		{"map[string][]string reshape when coerceMaps=true", map[string]interface{}{"k": []interface{}{"a", "b"}}, map[string][]string{}, true, map[string][]string{"k": {"a", "b"}}},
		{"map[string]interface{} reshape when coerceMaps=true", map[string]interface{}{"k": "v"}, map[string]interface{}{}, true, map[string]interface{}{"k": "v"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertToDefaultType(tc.value, tc.defaultValue, tc.coerceMaps)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}
