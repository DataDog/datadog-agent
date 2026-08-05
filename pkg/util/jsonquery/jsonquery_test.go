// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2019-present Datadog, Inc.

package jsonquery

import (
	"testing"
	"time"

	"github.com/DataDog/fastjq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/DataDog/datadog-agent/pkg/util/cache"
)

func TestJsonQueryParse(t *testing.T) {
	var program *fastjq.Program
	var err error

	program, err = Parse(".spec.foo")
	assert.NotNil(t, program)
	assert.NoError(t, err)
	value, found := cache.Cache.Get("jq-" + ".spec.foo")
	assert.True(t, found)
	assert.Equal(t, program, value)

	program, err = Parse(".$spec.foo")
	assert.Nil(t, program)
	assert.Error(t, err)
}

// TestJsonQueryParseCached checks that a second Parse of the same query returns the
// very same program rather than recompiling it.
func TestJsonQueryParseCached(t *testing.T) {
	first, err := Parse(".cached.query")
	require.NoError(t, err)
	second, err := Parse(".cached.query")
	require.NoError(t, err)
	assert.Same(t, first, second)
}

func TestQueryRun(t *testing.T) {
	object := map[string]interface{}{
		"foo": "bar",
		"baz": []interface{}{"toto", "titi"},
	}

	value, hasValue, err := RunSingleOutput(".foo", object)
	assert.Equal(t, "bar", value)
	assert.True(t, hasValue)
	assert.NoError(t, err)

	value, hasValue, err = RunSingleOutput(".bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.NoError(t, err)

	value, hasValue, err = RunSingleOutput(".%bar", object)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
	assert.Error(t, err)
}

func TestRunSingleOutputScalars(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		object   interface{}
		expected string
		hasValue bool
	}{
		{
			name:     "string is unquoted",
			query:    ".foo",
			object:   map[string]interface{}{"foo": "bar"},
			expected: "bar",
			hasValue: true,
		},
		{
			name:     "string keeps inner quotes and backslashes",
			query:    ".foo",
			object:   map[string]interface{}{"foo": `a "quoted" \ value`},
			expected: `a "quoted" \ value`,
			hasValue: true,
		},
		{
			// json.Marshal would escape < > & by default; the query output must not.
			name:     "string keeps HTML-significant characters verbatim",
			query:    ".foo",
			object:   map[string]interface{}{"foo": "a<b&c>d"},
			expected: "a<b&c>d",
			hasValue: true,
		},
		{
			name:     "unicode round-trips",
			query:    ".foo",
			object:   map[string]interface{}{"foo": "日本語"},
			expected: "日本語",
			hasValue: true,
		},
		{
			name:     "integer",
			query:    ".n",
			object:   map[string]interface{}{"n": 42},
			expected: "42",
			hasValue: true,
		},
		{
			name:     "float",
			query:    ".n",
			object:   map[string]interface{}{"n": 1.5},
			expected: "1.5",
			hasValue: true,
		},
		{
			name:     "boolean",
			query:    ".b",
			object:   map[string]interface{}{"b": false},
			expected: "false",
			hasValue: true,
		},
		{
			name:     "explicit null is reported as absent",
			query:    ".n",
			object:   map[string]interface{}{"n": nil},
			expected: "",
			hasValue: false,
		},
		{
			name:     "missing key is reported as absent",
			query:    ".nope",
			object:   map[string]interface{}{"foo": "bar"},
			expected: "",
			hasValue: false,
		},
		{
			name:     "empty output is reported as absent",
			query:    "empty",
			object:   map[string]interface{}{"foo": "bar"},
			expected: "",
			hasValue: false,
		},
		{
			// A nil object marshals to `null`; jq indexes null without erroring.
			name:     "nil object",
			query:    ".foo",
			object:   nil,
			expected: "",
			hasValue: false,
		},
		{
			name:     "computed boolean",
			query:    `.foo == "bar"`,
			object:   map[string]interface{}{"foo": "bar"},
			expected: "true",
			hasValue: true,
		},
		{
			name:     "nested access",
			query:    ".a.b[1]",
			object:   map[string]interface{}{"a": map[string]interface{}{"b": []interface{}{"x", "y"}}},
			expected: "y",
			hasValue: true,
		},
		{
			name:     "first of several outputs wins",
			query:    ".baz[]",
			object:   map[string]interface{}{"baz": []interface{}{"toto", "titi"}},
			expected: "toto",
			hasValue: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value, hasValue, err := RunSingleOutput(tt.query, tt.object)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, value)
			assert.Equal(t, tt.hasValue, hasValue)
		})
	}
}

// TestRunSingleOutputComposite pins the representation of non-scalar outputs. gojq
// rendered these through fmt.Sprint, which produced Go's map/slice syntax; fastjq
// returns them as JSON.
func TestRunSingleOutputComposite(t *testing.T) {
	object := map[string]interface{}{
		"obj": map[string]interface{}{"b": 1},
		"arr": []interface{}{1, "two"},
	}

	value, hasValue, err := RunSingleOutput(".obj", object)
	require.NoError(t, err)
	assert.True(t, hasValue)
	assert.JSONEq(t, `{"b":1}`, value)

	value, hasValue, err = RunSingleOutput(".arr", object)
	require.NoError(t, err)
	assert.True(t, hasValue)
	assert.JSONEq(t, `[1,"two"]`, value)
}

func TestRunSingleOutputRuntimeError(t *testing.T) {
	// Indexing a string is a jq runtime error, not an empty result.
	value, hasValue, err := RunSingleOutput(".foo.bar", map[string]interface{}{"foo": "bar"})
	assert.Error(t, err)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
}

func TestRunSingleOutputUnmarshalableObject(t *testing.T) {
	// Channels cannot be encoded as JSON, so the query never runs.
	value, hasValue, err := RunSingleOutput(".foo", map[string]interface{}{"foo": make(chan int)})
	assert.Error(t, err)
	assert.Equal(t, "", value)
	assert.False(t, hasValue)
}

var yamlTest = `
apiVersion: kubelet.config.k8s.io/v1beta1
authentication:
  anonymous:
    enabled: false
  webhook:
    cacheTTL: 0s
    enabled: foobar
  x509:
    clientCAFile: /etc/kubernetes/pki/ca.crt
authorization:
  mode: Webhook
  webhook:
    cacheAuthorizedTTL: 0s
    cacheUnauthorizedTTL: 0s
`

func TestYAML(t *testing.T) {
	var yamlContent interface{}
	err := yaml.Unmarshal([]byte(yamlTest), &yamlContent)
	assert.NoError(t, err)
	yamlContent = NormalizeYAMLForJQ(yamlContent)

	value, _, err := RunSingleOutput(".authentication.anonymous.enabled", yamlContent)
	assert.NoError(t, err)
	assert.Equal(t, "false", value)
}

// TestNormalizeYAMLForJQ covers the two conversions that make YAML output encodable as
// JSON: non-string map keys and time.Time values.
func TestNormalizeYAMLForJQ(t *testing.T) {
	timestamp := time.Date(2021, 1, 2, 15, 4, 5, 0, time.UTC)
	normalized := NormalizeYAMLForJQ(map[interface{}]interface{}{
		"str": "value",
		1:     "int key",
		"nested": []interface{}{
			map[interface{}]interface{}{true: "bool key"},
		},
		"ts": timestamp,
	})

	jsonData, err := marshalJSON(normalized)
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"str":"value","1":"int key","nested":[{"true":"bool key"}],"ts":"2021-01-02T15:04:05Z"}`,
		string(jsonData))
}
