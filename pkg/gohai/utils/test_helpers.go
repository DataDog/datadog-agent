// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build test

package utils

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AssertDecodedValue asserts that either
// - the field is an error and the decoded string is empty
// - the field is not an error and the value with its unit is equal to the decoded string
//
// decoded should be a string read by unmarshalling a json, and value the original Value[T]
// which corresponds to that string.
func AssertDecodedValue[T any](t *testing.T, decoded string, value *Value[T], unit string) {
	t.Helper()

	val, err := value.Value()
	if err == nil {
		// if the field is a real value then it should have the following format once rendered
		expected := fmt.Sprintf("%v%s", val, unit)
		assert.Equal(t, expected, decoded)
	} else {
		// if the field is an error then the json string associated to that field should be empty
		assert.Empty(t, decoded)
	}
}

// RequireMarshallJSON checks that
// - calling the AsJSON function succeeds
// - the object returned generates a json without error
// - the JSON can be unmarshalled into the given decoded struct
// - the JSON doesn't contain unexpected fields
func RequireMarshallJSON[T Jsonable, U any](t *testing.T, info T, decoded *U) {
	t.Helper()

	marshallable, _, err := info.AsJSON()
	require.NoError(t, err)

	marshalled, err := json.Marshal(marshallable)
	require.NoError(t, err)

	decoder := json.NewDecoder(bytes.NewReader(marshalled))
	// do not ignore unknown fields
	decoder.DisallowUnknownFields()

	err = decoder.Decode(decoded)
	require.NoError(t, err)

	// check that we read the full json
	require.False(t, decoder.More())
}
