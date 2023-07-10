// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertDecodedValue asserts that either
// - the field is an error and the decoded string is empty
// - the field is not an error and the value with its unit is equal to the decoded string
//
// decoded should be a string read by unmarshalling a json, and value the original Value[T]
// which corresponds to that string.
func AssertDecodedValue[T any](t *testing.T, decoded string, value *Value[T], unit string) {
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
