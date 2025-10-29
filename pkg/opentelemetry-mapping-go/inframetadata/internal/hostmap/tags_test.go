// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostmap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestGetHostAliases(t *testing.T) {
	var uninitializedSlice []string
	tests := []struct {
		name     string
		attrs    func() pcommon.Map
		expected []string
	}{
		{
			name: "no attributes",
			attrs: func() pcommon.Map {
				return pcommon.NewMap()
			},
			expected: uninitializedSlice,
		},
		{
			name: "attribute not present",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("some.other.key", "value")
				return m
			},
			expected: uninitializedSlice,
		},
		{
			name: "host aliases present",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				slice := m.PutEmptySlice(hostAliasAttribute)
				slice.AppendEmpty().SetStr("alias1")
				slice.AppendEmpty().SetStr("alias2")
				return m
			},
			expected: []string{"alias1", "alias2"},
		},
		{
			name: "host aliases present but with mixed types",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				slice := m.PutEmptySlice(hostAliasAttribute)
				slice.AppendEmpty().SetStr("alias1")
				slice.AppendEmpty().SetInt(123) // invalid, will be skipped
				slice.AppendEmpty().SetStr("alias2")
				return m
			},
			expected: []string{"alias1", "alias2"},
		},
		{
			name: "wrong type, no panic",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr(hostAliasAttribute, "alias")
				return m
			},
			expected: uninitializedSlice,
		},
		{
			name: "empty slice",
			attrs: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutEmptySlice(hostAliasAttribute)
				return m
			},
			expected: uninitializedSlice,
		},
		{
			name: "non initialized slice",
			attrs: func() pcommon.Map {
				var nonInitializedSlice []string
				m := pcommon.NewMap()
				m.FromRaw(map[string]any{hostAliasAttribute: nonInitializedSlice})
				return m
			},
			expected: uninitializedSlice,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getHostAliases(tt.attrs())
			assert.Equal(t, tt.expected, result)
		})
	}
}
