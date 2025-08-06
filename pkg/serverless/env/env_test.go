// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package env

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsAzureAppServicesExtension(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		envSet   bool
		expected bool
	}{
		{
			name:     "environment variable not set",
			envSet:   false,
			expected: false,
		},
		{
			name:     "environment variable set to '1'",
			envValue: "1",
			envSet:   true,
			expected: true,
		},
		{
			name:     "environment variable set to 'true'",
			envValue: "true",
			envSet:   true,
			expected: false,
		},
		{
			name:     "environment variable set to '0'",
			envValue: "0",
			envSet:   true,
			expected: false,
		},
		{
			name:     "environment variable set to empty string",
			envValue: "",
			envSet:   true,
			expected: false,
		},
		{
			name:     "environment variable set to other value",
			envValue: "something",
			envSet:   true,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envSet {
				t.Setenv(AzureAppServicesEnvVar, tt.envValue)
			}
			result := IsAzureAppServicesExtension()
			assert.Equal(t, tt.expected, result)
		})
	}
}

