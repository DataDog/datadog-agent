// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// AssertDecodedValue asserts that either the value is an error and the decoded string is empty, or
// that the value with its unit is equal to the decoded string
func AssertDecodedValue[T any](t *testing.T, decoded string, value *Value[T], unit string) {
	val, err := value.Value()
	if err == nil {
		expected := fmt.Sprintf("%v%s", val, unit)
		assert.Equal(t, expected, decoded)
	} else {
		assert.Empty(t, decoded)
	}
}
