// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package jsonquery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// Data contains YAML code
type data []byte

func TestYAMLExistQuery(t *testing.T) {
	exist, err := YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.50\"")
	assert.NoError(t, err)
	assert.True(t, exist)

	exist, err = YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)

	exist, err = YAMLCheckExist(data("{\"ip_address\": \"127.0.0.50\"}"), ".ip_address")
	assert.EqualError(t, err, "filter query must return a boolean: yaml: unmarshal errors:\n  line 1: cannot unmarshal !!str `127.0.0.50` into bool")
	assert.False(t, exist)

	exist, err = YAMLCheckExist(data("{}"), ".ip_address == \"127.0.0.99\"")
	assert.NoError(t, err)
	assert.False(t, exist)
}

func TestNormalizeYAMLForGoJQ(t *testing.T) {
	// map[interface{}]interface{} gets converted to map[string]interface{}
	input := map[interface{}]interface{}{
		"key":  "value",
		42:     "number-key",
		"nest": map[interface{}]interface{}{"inner": "val"},
	}
	result := NormalizeYAMLForGoJQ(input)
	m, ok := result.(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "value", m["key"])
	assert.Equal(t, "number-key", m["42"])
	inner, ok := m["nest"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "val", inner["inner"])

	// map[string]interface{} recurses into values
	input2 := map[string]interface{}{
		"a": map[interface{}]interface{}{"b": "c"},
	}
	result2 := NormalizeYAMLForGoJQ(input2)
	m2 := result2.(map[string]interface{})
	nested := m2["a"].(map[string]interface{})
	assert.Equal(t, "c", nested["b"])

	// []interface{} normalizes elements
	input3 := []interface{}{
		map[interface{}]interface{}{"x": "y"},
		"plain",
	}
	result3 := NormalizeYAMLForGoJQ(input3)
	slice := result3.([]interface{})
	assert.Len(t, slice, 2)
	elem := slice[0].(map[string]interface{})
	assert.Equal(t, "y", elem["x"])
	assert.Equal(t, "plain", slice[1])

	// time.Time gets formatted as RFC3339
	ts := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	result4 := NormalizeYAMLForGoJQ(ts)
	assert.Equal(t, "2024-06-15T12:00:00Z", result4)

	// other types pass through
	assert.Equal(t, 42, NormalizeYAMLForGoJQ(42))
	assert.Equal(t, "hello", NormalizeYAMLForGoJQ("hello"))
}
