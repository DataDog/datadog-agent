// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStringFromBytes(t *testing.T) {
	// null-terminated string
	assert.Equal(t, "hello", StringFromBytes([]byte{'h', 'e', 'l', 'l', 'o', 0}))

	// no null terminator
	assert.Equal(t, "hello", StringFromBytes([]byte{'h', 'e', 'l', 'l', 'o'}))

	// null byte in the middle truncates
	assert.Equal(t, "he", StringFromBytes([]byte{'h', 'e', 0, 'l', 'o'}))

	// empty slice
	assert.Equal(t, "", StringFromBytes([]byte{}))

	// just a null byte
	assert.Equal(t, "", StringFromBytes([]byte{0}))
}
