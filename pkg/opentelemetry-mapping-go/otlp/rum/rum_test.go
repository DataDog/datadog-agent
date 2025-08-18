// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package rum

import (
	"net/url"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			name:  "string value",
			key:   "test.key",
			value: pcommon.NewValueStr("test_value"),
			expected: map[string]any{
				"test": map[string]any{
					"key": "test_value",
				},
			},
		},
		{
			name:  "int value",
			key:   "count",
			value: pcommon.NewValueInt(42),
			expected: map[string]any{
				"count": int64(42),
			},
		},
		{
			name:  "double value",
			key:   "score",
			value: pcommon.NewValueDouble(3.14),
			expected: map[string]any{
				"score": 3.14,
			},
		},
		{
			name:  "bool value",
			key:   "enabled",
			value: pcommon.NewValueBool(true),
			expected: map[string]any{
				"enabled": true,
			},
		},
		{
			name:  "deep nested structure",
			key:   "a.b.c.d.e",
			value: pcommon.NewValueStr("test_value"),
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
			name:  "empty key",
			key:   "",
			value: pcommon.NewValueStr("empty_key_value"),
			expected: map[string]any{
				"": "empty_key_value",
			},
		},
		{
			name:  "nil map value",
			key:   "test.key",
			value: pcommon.NewValueMap(),
			expected: map[string]any{
				"test": map[string]any{
					"key": nil,
				},
			},
		},
		{
			name:  "empty bytes value",
			key:   "test.key",
			value: pcommon.NewValueBytes(),
			expected: map[string]any{
				"test": map[string]any{
					"key": nil,
				},
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
				"service.name":    pcommon.NewValueStr("test-service"),
				"service.version": pcommon.NewValueStr("1.0.0"),
			},
			expected: map[string]any{
				"service": "test-service",
				"version": "1.0.0",
			},
		},
		{
			name: "datadog prefixed attributes",
			attrs: map[string]pcommon.Value{
				"datadog.custom.attr": pcommon.NewValueStr("custom_value"),
				"datadog.nested.attr": pcommon.NewValueInt(42),
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
				"datadog.enabled": pcommon.NewValueBool(true),
				"datadog.count":   pcommon.NewValueInt(100),
				"datadog.score":   pcommon.NewValueDouble(95.5),
			},
			expected: map[string]any{
				"enabled": true,
				"count":   int64(100),
				"score":   95.5,
			},
		},
		{
			name: "empty string values",
			attrs: map[string]pcommon.Value{
				"datadog.empty": pcommon.NewValueStr(""),
			},
			expected: map[string]any{
				"empty": "",
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
}

func TestParseDDForwardIntoResource(t *testing.T) {
	tests := []struct {
		name      string
		ddforward string
		expected  pcommon.Map
	}{
		{
			name:      "empty ddforward",
			ddforward: "",
			expected:  pcommon.NewMap(),
		},
		{
			name:      "successful parse of ddforward",
			ddforward: "/api/v2/rum?ddsource=browser&ddtags=sdk_version:4.41.0,env:prod,service:test-app,version:2.0.0-beta&dd-evp-origin=browser&dd-request-id=1234-5678-91a-bcde&batch_time=1682595634052&dd-api-key=1234567890",
			expected: func() pcommon.Map {
				m := pcommon.NewMap()
				m.PutStr("batch_time", "1682595634052")
				m.PutStr("ddsource", "browser")

				ddtags := m.PutEmptyMap("ddtags")
				ddtags.PutStr("sdk_version", "4.41.0")
				ddtags.PutStr("env", "prod")
				ddtags.PutStr("service", "test-app")
				ddtags.PutStr("version", "2.0.0-beta")

				m.PutStr("dd-evp-origin", "browser")
				m.PutStr("dd-request-id", "1234-5678-91a-bcde")
				m.PutStr("dd-api-key", "1234567890")
				return m
			}(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attributes := pcommon.NewMap()
			parseDDForwardIntoResource(attributes, tt.ddforward)
			tt.expected.Range(func(key string, expectedValue pcommon.Value) bool {
				actualValue, _ := attributes.Get(key)
				if key == "ddtags" {
					expectedValue.Map().Range(func(mapKey string, mapValue pcommon.Value) bool {
						actualDDTagsValue, _ := actualValue.Map().Get(mapKey)
						assert.Equal(t, mapValue.AsString(), actualDDTagsValue.AsString())
						return true
					})
				} else {
					assert.Equal(t, expectedValue.AsString(), actualValue.AsString())
				}
				return true
			})
		})
	}
}

func TestBuildIntakeUrlPathAndParameters(t *testing.T) {
	tests := []struct {
		name   string
		rattrs pcommon.Map
		lattrs pcommon.Map
		want   string
	}{
		{
			name: "successfully build intake url path and parameters",
			rattrs: func() pcommon.Map {
				rattrs := pcommon.NewMap()
				rattrs.PutStr("batch_time", "123")
				ddTagsMap := rattrs.PutEmptyMap("ddtags")
				ddTagsMap.PutStr("service", "service-rattrs")
				ddTagsMap.PutStr("env", "prod")
				ddTagsMap.PutStr("sdk_version", "1.2.3")
				ddTagsMap.PutStr("version", "1.2.3")
				rattrs.PutStr("ddsource", "browser")
				rattrs.PutStr("dd-evp-origin", "browser")
				rattrs.PutStr("dd-request-id", "456")
				rattrs.PutStr("dd-api-key", "1234567890")
				return rattrs
			}(),
			lattrs: func() pcommon.Map {
				lattrs := pcommon.NewMap()
				serviceMap := lattrs.PutEmptyMap("service")
				serviceMap.PutStr("name", "service")
				serviceMap.PutStr("version", "1.2.3")
				return lattrs
			}(),
			want: "/api/v2/rum?batch_time=123&ddtags=env:prod,sdk_version:1.2.3,service:service,version:1.2.3&ddsource=browser&dd-evp-origin=browser&dd-request-id=456&dd-api-key=1234567890",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildIntakeUrlPathAndParameters(tt.rattrs, tt.lattrs)
			uri, err := url.Parse(got)
			require.NoError(t, err)
			queryParams := uri.Query()

			// handle ddtags specially - split, sort, and rejoin
			var ddtagsStr string
			if queryParams.Get("ddtags") != "" {
				ddtags := strings.Split(queryParams.Get("ddtags"), ",")
				sort.Strings(ddtags)
				ddtagsStr = strings.Join(ddtags, ",")
			}

			var queryParts []string
			queryParts = append(queryParts, "batch_time="+queryParams.Get("batch_time"))

			if ddtagsStr != "" {
				queryParts = append(queryParts, "ddtags="+ddtagsStr)
			}

			queryParts = append(queryParts, "ddsource="+queryParams.Get("ddsource"))
			queryParts = append(queryParts, "dd-evp-origin="+queryParams.Get("dd-evp-origin"))
			queryParts = append(queryParts, "dd-request-id="+queryParams.Get("dd-request-id"))
			queryParts = append(queryParts, "dd-api-key="+queryParams.Get("dd-api-key"))

			reconstructedURL := "/api/v2/rum?" + strings.Join(queryParts, "&")

			assert.Equal(t, tt.want, reconstructedURL)
		})
	}
}
