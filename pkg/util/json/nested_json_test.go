// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package json

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetNestedValueExists(t *testing.T) {
	rawJSON := []byte(`{"key":"val"}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, "val", GetNestedValue(jsonMap, "key"))
}

func TestGetNestedValueExistsNested(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key2": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, "val2", GetNestedValue(jsonMap, "key2", "key3", "key4"))
}

func TestGetNestedValueExistsStruct(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key2": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, map[string]interface{}{
		"key4": "val2",
	}, GetNestedValue(jsonMap, "key2", "key3"))
}

func TestGetNestedValueDoesntExist(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key5": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, nil, GetNestedValue(jsonMap, "doesnt_exist", "key3"))
}

func TestGetNestedValueDoesntExistNested(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key5": {"key3": {"key4": "val2"}}}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, nil, GetNestedValue(jsonMap, "key5", "doesnt_exist"))
}

func TestGetNestedValueExistsEarly(t *testing.T) {
	rawJSON := []byte(`{"key":"val", "key2": "val"}`)
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(rawJSON, &jsonMap)
	assert.NoError(t, err)

	assert.Equal(t, nil, GetNestedValue(jsonMap, "key2", "key1"))
}
