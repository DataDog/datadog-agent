// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package utils

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

//go:embed fixtures/tags_a.yaml
var tagsA string

//go:embed fixtures/tags_b.yaml
var tagsB string

//go:embed fixtures/tags_ab.yaml
var tagsAB string

func TestMergeYAML(t *testing.T) {
	tests := map[string]struct {
		oldValues      string
		newValues      string
		expectedResult string
		expectError    bool
	}{
		"no new values":                          {oldValues: "a: 1\nb: 2", newValues: "", expectedResult: "a: 1\nb: 2", expectError: false},
		"no old values":                          {oldValues: "", newValues: "a: 1\nb: 2", expectedResult: "a: 1\nb: 2", expectError: false},
		"old value not valid yaml":               {oldValues: "- a:b:", newValues: "a: 1\nb: 2", expectedResult: "", expectError: true},
		"new value not valid yaml":               {oldValues: "a: 1\nb: 2", newValues: "- a:b:", expectedResult: "", expectError: true},
		"golden path":                            {oldValues: "a: 1", newValues: "b: 2", expectedResult: "a: 1\nb: 2\n", expectError: false},
		"slices value, no merge slices":          {oldValues: tagsA, newValues: tagsB, expectedResult: tagsB, expectError: false},
		"slices value inverted, no merge slices": {oldValues: tagsB, newValues: tagsA, expectedResult: tagsA, expectError: false},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := MergeYAML(tc.oldValues, tc.newValues)
			if tc.expectError {
				require.Error(t, err, "expected error, got nil")
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
			}

			var gotYAML map[string]interface{}
			var expectedYAML map[string]interface{}

			gotMap := yaml.Unmarshal([]byte(got), &gotYAML)
			expectedMap := yaml.Unmarshal([]byte(tc.expectedResult), &expectedYAML)
			assert.Equal(t, gotMap, expectedMap, "expected %v, got %v", expectedMap, gotMap)
		})
	}
}

func TestMergeYAMLWithSlices(t *testing.T) {
	tests := map[string]struct {
		oldValues      string
		newValues      string
		expectedResult string
		expectError    bool
	}{
		"no new values, merge slices":            {oldValues: "a: 1\nb: 2", newValues: "", expectedResult: "a: 1\nb: 2", expectError: false},
		"no old values, merge slices":            {oldValues: "", newValues: "a: 1\nb: 2", expectedResult: "a: 1\nb: 2", expectError: false},
		"old value not valid yaml, merge slices": {oldValues: "- a:b:", newValues: "a: 1\nb: 2", expectedResult: "", expectError: true},
		"new value not valid yaml, merge slices": {oldValues: "a: 1\nb: 2", newValues: "- a:b:", expectedResult: "", expectError: true},
		"golden path, merge slices":              {oldValues: "a: 1", newValues: "b: 2", expectedResult: "a: 1\nb: 2\n", expectError: false},
		"slices value, merge slices":             {oldValues: tagsA, newValues: tagsB, expectedResult: tagsAB, expectError: false},
		"slices value inverted, merge slices":    {oldValues: tagsB, newValues: tagsA, expectedResult: tagsAB, expectError: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := MergeYAMLWithSlices(tc.oldValues, tc.newValues)
			if tc.expectError {
				require.Error(t, err, "expected error, got nil")
			} else {
				require.NoError(t, err, "unexpected error: %v", err)
			}

			var gotYAML map[string]interface{}
			var expectedYAML map[string]interface{}
			err = yaml.Unmarshal([]byte(got), &gotYAML)
			require.NoError(t, err, "unexpected error: %v", err)
			err = yaml.Unmarshal([]byte(tc.expectedResult), &expectedYAML)
			require.NoError(t, err, "unexpected error: %v", err)
			assertMapsEqual(t, expectedYAML, gotYAML)
		})
	}
}

func assertMapsEqual(t *testing.T, expected, actual map[string]interface{}) {
	assert.Equal(t, len(expected), len(actual), "expected %v, got %v", expected, actual)
	for k, v := range expected {
		switch v.(type) {
		case map[string]interface{}:
			assertMapsEqual(t, v.(map[string]interface{}), actual[k].(map[string]interface{}))
		case []interface{}:
			assert.ElementsMatch(t, v.([]interface{}), actual[k].([]interface{}), "expected %v, got %v", v, actual[k])
		default:
			assert.Equal(t, v, actual[k], "expected %v, got %v", v, actual[k])
		}
	}
}
