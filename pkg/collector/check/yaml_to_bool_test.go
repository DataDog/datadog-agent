// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestYAMLToBool(t *testing.T) {
	// The spellings yaml v2 resolved to booleans, kept for unquoted values in
	// verbatim YAML configs.
	for _, tc := range []struct {
		value interface{}
		want  bool
	}{
		{true, true},
		{false, false},
		{"yes", true},
		{"on", true},
		{"no", false},
		{"off", false},
	} {
		b, ok := YAMLToBool(tc.value)
		assert.True(t, ok, "expected %v to be accepted", tc.value)
		assert.Equal(t, tc.want, b, "expected %v to be %v", tc.value, tc.want)
	}

	// Everything else is rejected: case variants, single letters, string
	// booleans and non-boolean values keep their string meaning.
	for _, value := range []interface{}{
		nil,
		"Yes", "YES", "On", "ON", "No", "NO", "Off", "OFF",
		"y", "Y", "n", "N",
		"true", "false", "True", "TRUE", "False", "FALSE",
		"maybe", "0", "1",
		0, 1,
		[]interface{}{}, map[string]interface{}{},
	} {
		_, ok := YAMLToBool(value)
		assert.False(t, ok, "expected %v to be rejected", value)
	}
}
