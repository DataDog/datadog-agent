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
