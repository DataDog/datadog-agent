// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewEntityID(t *testing.T) {
	goodPrefixes := AllPrefixesSet()
	badPrefixes := []string{
		"",
		"bad_prefix",
	}

	for good := range goodPrefixes {
		NewEntityID(good, "12345")
	}

	for _, bad := range badPrefixes {
		assert.Panics(t, func() { NewEntityID(EntityIDPrefix(bad), "12345") }, "Expected a panic to happen")
	}

}

func TestExtractPrefixAndID(t *testing.T) {
	testCases := []struct {
		name           string
		input          string
		expectedPrefix EntityIDPrefix
		expectedID     string
		expectError    bool
	}{

		{
			name:           "proper input",
			input:          "container_id://123456",
			expectedPrefix: ContainerID,
			expectedID:     "123456",
			expectError:    false,
		},

		{
			name:           "malformatted input",
			input:          "container_id:123456",
			expectedPrefix: "",
			expectedID:     "",
			expectError:    true,
		},

		{
			name:           "good format, but unsupported prefix",
			input:          "bad-prefix://123456",
			expectedPrefix: "",
			expectedID:     "",
			expectError:    true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			prefix, id, err := ExtractPrefixAndID(testCase.input)

			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, testCase.expectedPrefix, prefix)
			assert.Equal(t, testCase.expectedID, id)
		})
	}
}
