// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToLenientBool(t *testing.T) {
	// The spellings and their case variants are the ones go.yaml.in/yaml/v2
	// resolved to booleans, which yaml v4 only keeps when unmarshaling into
	// typed bool fields. Generic map reads need this helper to keep the same
	// behavior for unquoted spellings in verbatim YAML configs.
	for _, value := range []interface{}{
		true,
		"yes", "Yes", "YES",
		"on", "On", "ON",
		"y", "Y",
	} {
		b, ok := ToLenientBool(value)
		assert.True(t, ok, "expected %v to be accepted", value)
		assert.True(t, b, "expected %v to be true", value)
	}

	for _, value := range []interface{}{
		false,
		"no", "No", "NO",
		"off", "Off", "OFF",
		"n", "N",
	} {
		b, ok := ToLenientBool(value)
		assert.True(t, ok, "expected %v to be accepted", value)
		assert.False(t, b, "expected %v to be false", value)
	}

	// String "true"/"false" are deliberately rejected: a quoted boolean or a
	// JSON string value (e.g. in a container or Kubernetes annotation) was
	// left as a string and ignored by the yaml v2 generic map reads, and fleet
	// data shows many customers with such values on disk check options.
	for _, value := range []interface{}{
		nil,
		"true", "True", "TRUE",
		"false", "False", "FALSE",
		"maybe", "enabled", "0", "1", "o", "offish", "yEs",
		0, 1,
		[]interface{}{}, map[string]interface{}{},
	} {
		_, ok := ToLenientBool(value)
		assert.False(t, ok, "expected %v to be rejected", value)
	}
}
