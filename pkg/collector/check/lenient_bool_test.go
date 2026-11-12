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
	// resolved to booleans (see its resolve.go), plus the plain booleans and
	// the true/false strings that go.yaml.in/yaml/v4 produces.
	for _, value := range []interface{}{
		true,
		"true", "True", "TRUE",
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
		"false", "False", "FALSE",
		"no", "No", "NO",
		"off", "Off", "OFF",
		"n", "N",
	} {
		b, ok := ToLenientBool(value)
		assert.True(t, ok, "expected %v to be accepted", value)
		assert.False(t, b, "expected %v to be false", value)
	}

	// Values that are not boolean spellings are rejected, not guessed from:
	// they keep their string meaning.
	for _, value := range []interface{}{
		nil,
		"maybe", "enabled", "0", "1", "o", "offish", "yEs",
		0, 1,
		[]interface{}{}, map[string]interface{}{},
	} {
		_, ok := ToLenientBool(value)
		assert.False(t, ok, "expected %v to be rejected", value)
	}
}
