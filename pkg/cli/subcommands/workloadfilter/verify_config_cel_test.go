// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel

package workloadfilterlist

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyCELConfig_ValidConfigurations(t *testing.T) {
	testCases := []struct {
		name     string
		filename string
	}{
		{
			name:     "Valid configuration with multiple products (YAML)",
			filename: "valid_multiple_products.yaml",
		},
		{
			name:     "Valid configuration with multiple products (JSON)",
			filename: "valid_multiple_products.json",
		},
		{
			name:     "Valid complex configuration with grouped logic (YAML)",
			filename: "valid_complex.yaml",
		},
		{
			name:     "Valid complex configuration (JSON)",
			filename: "valid_complex.json",
		},
		{
			name:     "Valid configuration covering all resource types (JSON)",
			filename: "valid_all_resources.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testFile := filepath.Join("testdata", tc.filename)
			file, err := os.Open(testFile)
			require.NoError(t, err, "Failed to open test file")
			defer file.Close()

			var output bytes.Buffer
			err = verifyCELConfig(&output, file)
			assert.NoError(t, err, "Expected valid configuration to pass")
			assert.Contains(t, output.String(), "All rules are valid!", "Expected success message")
		})
	}
}

func TestVerifyCELConfig_ErrorCases(t *testing.T) {
	testCases := []struct {
		name          string
		filename      string
		expectedError string
	}{
		{
			name:          "Invalid CEL syntax (YAML)",
			filename:      "error_invalid_syntax.yaml",
			expectedError: "CEL compilation failed",
		},
		{
			name:          "Invalid field reference (JSON)",
			filename:      "error_invalid_field.json",
			expectedError: "CEL compilation failed",
		},
		{
			name:          "Invalid product name (YAML)",
			filename:      "error_invalid_product.yaml",
			expectedError: "invalid configuration structure",
		},
		{
			name:          "Malformed YAML",
			filename:      "error_malformed.yaml",
			expectedError: "failed to parse input",
		},
		{
			name:          "Malformed JSON",
			filename:      "error_malformed.json",
			expectedError: "failed to parse input",
		},
		{
			name:          "Empty rules (YAML)",
			filename:      "error_empty_rules.yaml",
			expectedError: "no rules found in the input",
		},
		{
			name:          "Type mismatch in CEL expression (YAML)",
			filename:      "error_type_mismatch.yaml",
			expectedError: "CEL compilation failed",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			testFile := filepath.Join("testdata", tc.filename)
			file, err := os.Open(testFile)
			require.NoError(t, err, "Failed to open test file")
			defer file.Close()

			var output bytes.Buffer
			err = verifyCELConfig(&output, file)
			assert.Error(t, err, "Expected configuration to fail")
			assert.Contains(t, err.Error(), tc.expectedError, "Expected specific error message")
		})
	}
}
