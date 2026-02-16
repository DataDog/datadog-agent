// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tags

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAddTagInvalidNoValue(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "invalidTag")
	assert.Equal(t, 2, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
}

func TestAddTagInvalidEmpty(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "")
	assert.Equal(t, 2, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
}

func TestAddTagValid(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "VaLiD:TaG")
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
	assert.Equal(t, "tag", tagMap["valid"])
}

func TestAddTagValidWithColumnInValue(t *testing.T) {
	tagMap := map[string]string{
		"key_a": "value_a",
		"key_b": "value_b",
	}
	addTag(tagMap, "VaLiD:TaG:Val")
	assert.Equal(t, 3, len(tagMap))
	assert.Equal(t, "value_a", tagMap["key_a"])
	assert.Equal(t, "value_b", tagMap["key_b"])
	assert.Equal(t, "tag:val", tagMap["valid"])
}

func TestArrayToMap(t *testing.T) {
	result := ArrayToMap([]string{"env:prod,service:web"})
	assert.Equal(t, "prod", result["env"])
	assert.Equal(t, "web", result["service"])
}

func TestArrayToMapMultipleEntries(t *testing.T) {
	result := ArrayToMap([]string{"env:staging", "region:us-east"})
	assert.Equal(t, "staging", result["env"])
	assert.Equal(t, "us-east", result["region"])
}

func TestArrayToMapEmpty(t *testing.T) {
	result := ArrayToMap([]string{})
	assert.Empty(t, result)
}

func TestMapToArray(t *testing.T) {
	input := map[string]string{"env": "prod", "service": "web"}
	result := MapToArray(input)
	assert.Len(t, result, 2)
	assert.Contains(t, result, "env:prod")
	assert.Contains(t, result, "service:web")
}

func TestMapToArrayEmpty(t *testing.T) {
	result := MapToArray(map[string]string{})
	assert.Empty(t, result)
}

func TestMergeWithOverwrite(t *testing.T) {
	base := map[string]string{"env": "staging", "region": "us-east"}
	overwrite := map[string]string{"env": "prod", "service": "web"}
	result := MergeWithOverwrite(base, overwrite)

	assert.Equal(t, "prod", result["env"]) // overwritten
	assert.Equal(t, "us-east", result["region"])
	assert.Equal(t, "web", result["service"])

	// original maps are not mutated
	assert.Equal(t, "staging", base["env"])
}
