// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkconfig

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_normalizeRegexReplaceValue(t *testing.T) {
	tests := []struct {
		val                   string
		expectedReplacedValue string
	}{
		{
			"abc",
			"abc",
		},
		{
			"a\\1b",
			"a$1b",
		},
		{
			"a$1b",
			"a$1b",
		},
		{
			"\\1",
			"$1",
		},
		{
			"\\2",
			"$2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			assert.Equal(t, tt.expectedReplacedValue, normalizeRegexReplaceValue(tt.val))
		})
	}
}

func Test_getMappedValue(t *testing.T) {
	tests := []struct {
		name                string
		val                 string
		mapping             map[string]string
		expectedMappedValue string
		expectedError       string
	}{
		{
			name: "one value",
			val:  "1",
			mapping: map[string]string{
				"1": "one",
			},
			expectedMappedValue: "one",
		},
		{
			name: "multiple values",
			val:  "2",
			mapping: map[string]string{
				"1": "one",
				"2": "two",
			},
			expectedMappedValue: "two",
		},
		{
			name: "invalid value",
			val:  "3",
			mapping: map[string]string{
				"1": "one",
				"2": "two",
			},
			expectedError: "mapping for `3` does not exist. mapping=`map[1:one 2:two]`",
		},
		{
			name:                "empty mapping",
			val:                 "4",
			mapping:             map[string]string{},
			expectedMappedValue: "4",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mappedValue, err := GetMappedValue(tt.val, tt.mapping)
			assert.Equal(t, tt.expectedMappedValue, mappedValue)
			if tt.expectedError != "" {
				assert.EqualError(t, err, tt.expectedError)
			}
		})
	}
}
