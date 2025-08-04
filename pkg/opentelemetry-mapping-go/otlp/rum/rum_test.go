// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rum

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestBuildRumPayload(t *testing.T) {
	tests := []struct {
		name     string
		key      string
		value    pcommon.Value
		expected map[string]any
	}{
		{
			name: "string value",
			key:  "test.key",
			value: func() pcommon.Value {
				v := pcommon.NewValueStr("test_value")
				return v
			}(),
			expected: map[string]any{
				"test": map[string]any{
					"key": "test_value",
				},
			},
		},
		{
			name: "int value",
			key:  "count",
			value: func() pcommon.Value {
				v := pcommon.NewValueInt(42)
				return v
			}(),
			expected: map[string]any{
				"count": int64(42),
			},
		},
		{
			name: "double value",
			key:  "score",
			value: func() pcommon.Value {
				v := pcommon.NewValueDouble(3.14)
				return v
			}(),
			expected: map[string]any{
				"score": 3.14,
			},
		},
		{
			name: "bool value",
			key:  "enabled",
			value: func() pcommon.Value {
				v := pcommon.NewValueBool(true)
				return v
			}(),
			expected: map[string]any{
				"enabled": true,
			},
		},
		{
			name: "deep nested structure",
			key:  "a.b.c.d.e",
			value: func() pcommon.Value {
				v := pcommon.NewValueStr("test_value")
				return v
			}(),
			expected: map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"d": map[string]any{
								"e": "test_value",
							},
						},
					},
				},
			},
		},
		{
			name: "slice value",
			key:  "tags",
			value: func() pcommon.Value {
				v := pcommon.NewValueSlice()
				slice := v.Slice()
				slice.AppendEmpty().SetStr("tag1")
				slice.AppendEmpty().SetStr("tag2")
				return v
			}(),
			expected: map[string]any{
				"tags": []any{"tag1", "tag2"},
			},
		},
		{
			name: "map value",
			key:  "metadata",
			value: func() pcommon.Value {
				v := pcommon.NewValueMap()
				m := v.Map()
				m.PutStr("key1", "value1")
				m.PutInt("key2", 123)
				return v
			}(),
			expected: map[string]any{
				"metadata": map[string]any{
					"key1": "value1",
					"key2": int64(123),
				},
			},
		},
		{
			name: "empty key",
			key:  "",
			value: func() pcommon.Value {
				v := pcommon.NewValueStr("empty_key_value")
				return v
			}(),
			expected: map[string]any{
				"": "empty_key_value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rumPayload := make(map[string]any)
			buildRumPayload(tt.key, tt.value, rumPayload)

			assert.Equal(t, tt.expected, rumPayload)
		})
	}

	t.Run("override existing value", func(t *testing.T) {
		rumPayload := make(map[string]any)

		buildRumPayload("test.key", pcommon.NewValueStr("original_value"), rumPayload)

		mapVal := pcommon.NewValueMap()
		mapVal.Map().PutStr("nested.key", "nested_value")
		buildRumPayload("test.key", mapVal, rumPayload)

		expected := map[string]any{
			"test": map[string]any{
				"key": map[string]any{
					"nested": map[string]any{
						"key": "nested_value",
					},
				},
			},
		}

		assert.Equal(t, expected, rumPayload)
	})
}

func TestConstructRumPayloadFromOTLP(t *testing.T) {
	tests := []struct {
		name     string
		attrs    map[string]pcommon.Value
		expected map[string]any
	}{
		{
			name:     "empty attributes",
			attrs:    map[string]pcommon.Value{},
			expected: map[string]any{},
		},
		{
			name: "mapped attributes",
			attrs: map[string]pcommon.Value{
				"service.name": func() pcommon.Value {
					v := pcommon.NewValueStr("test-service")
					return v
				}(),
				"service.version": func() pcommon.Value {
					v := pcommon.NewValueStr("1.0.0")
					return v
				}(),
			},
			expected: map[string]any{
				"service": "test-service",
				"version": "1.0.0",
			},
		},
		{
			name: "datadog prefixed attributes",
			attrs: map[string]pcommon.Value{
				"datadog.custom.attr": func() pcommon.Value {
					v := pcommon.NewValueStr("custom_value")
					return v
				}(),
				"datadog.nested.attr": func() pcommon.Value {
					v := pcommon.NewValueInt(42)
					return v
				}(),
			},
			expected: map[string]any{
				"custom": map[string]any{
					"attr": "custom_value",
				},
				"nested": map[string]any{
					"attr": int64(42),
				},
			},
		},
		{
			name: "nested maps and slices",
			attrs: map[string]pcommon.Value{
				"datadog.user.profile": func() pcommon.Value {
					v := pcommon.NewValueMap()
					m := v.Map()
					m.PutStr("name", "John Doe")
					m.PutInt("age", 30)
					return v
				}(),
				"datadog.tags": func() pcommon.Value {
					v := pcommon.NewValueSlice()
					slice := v.Slice()
					slice.AppendEmpty().SetStr("production")
					slice.AppendEmpty().SetStr("frontend")
					return v
				}(),
			},
			expected: map[string]any{
				"user": map[string]any{
					"profile": map[string]any{
						"name": "John Doe",
						"age":  int64(30),
					},
				},
				"tags": []any{"production", "frontend"},
			},
		},
		{
			name: "boolean, int and double values",
			attrs: map[string]pcommon.Value{
				"datadog.enabled": func() pcommon.Value {
					v := pcommon.NewValueBool(true)
					return v
				}(),
				"datadog.count": func() pcommon.Value {
					v := pcommon.NewValueInt(100)
					return v
				}(),
				"datadog.score": func() pcommon.Value {
					v := pcommon.NewValueDouble(95.5)
					return v
				}(),
			},
			expected: map[string]any{
				"enabled": true,
				"count":   int64(100),
				"score":   95.5,
			},
		},
		{
			name: "override existing values",
			attrs: map[string]pcommon.Value{
				"datadog.existing": func() pcommon.Value {
					v := pcommon.NewValueStr("original")
					return v
				}(),
				"datadog.existing.nested": func() pcommon.Value {
					v := pcommon.NewValueStr("nested_value")
					return v
				}(),
			},
			expected: map[string]any{
				"existing": map[string]any{
					"nested": "nested_value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrMap := pcommon.NewMap()
			for k, v := range tt.attrs {
				v.CopyTo(attrMap.PutEmpty(k))
			}

			result := ConstructRumPayloadFromOTLP(attrMap)

			assert.Equal(t, tt.expected, result)
		})
	}

	t.Run("null values", func(t *testing.T) {
		attrMap := pcommon.NewMap()
		nullVal := attrMap.PutEmpty("datadog.null.value")
		nullVal.SetEmptyBytes() // This creates a null value

		result := ConstructRumPayloadFromOTLP(attrMap)
		expected := map[string]any{
			"null": map[string]any{
				"value": nil,
			},
		}

		assert.Equal(t, expected, result)
	})

	t.Run("empty string values", func(t *testing.T) {
		attrMap := pcommon.NewMap()
		attrMap.PutStr("datadog.empty", "")

		result := ConstructRumPayloadFromOTLP(attrMap)
		expected := map[string]any{
			"empty": "",
		}

		assert.Equal(t, expected, result)
	})
}
