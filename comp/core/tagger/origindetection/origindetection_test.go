// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package origindetection contains the types and functions used for Origin Detection.
package origindetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseLocalData(t *testing.T) {
	tests := []struct {
		name         string
		rawLocalData string
		expected     LocalData
		expectError  bool
	}{
		{
			name:         "Empty string",
			rawLocalData: "",
			expected:     LocalData{},
			expectError:  false,
		},
		{
			name:         "Single container ID",
			rawLocalData: "ci-abc123",
			expected:     LocalData{ContainerID: "abc123"},
			expectError:  false,
		},
		{
			name:         "Single inode",
			rawLocalData: "in-12345",
			expected:     LocalData{Inode: 12345},
			expectError:  false,
		},
		{
			name:         "Multiple values",
			rawLocalData: "ci-abc123,in-12345",
			expected:     LocalData{ContainerID: "abc123", Inode: 12345},
			expectError:  false,
		},
		{
			name:         "Multiple values in reverse order",
			rawLocalData: "in-12345,ci-abc123",
			expected:     LocalData{ContainerID: "abc123", Inode: 12345},
			expectError:  false,
		},
		{
			name:         "Invalid inode",
			rawLocalData: "in-invalid",
			expected:     LocalData{},
			expectError:  true,
		},
		{
			name:         "Old APM container format",
			rawLocalData: "cid-abc123",
			expected:     LocalData{ContainerID: "abc123"},
			expectError:  false,
		},
		{
			name:         "Old DogStatsD container format",
			rawLocalData: "abc123",
			expected:     LocalData{ContainerID: "abc123"},
			expectError:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseLocalData(tc.rawLocalData)

			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tc.expected, result)
			}

		})
	}
}

func TestParseExternalData(t *testing.T) {
	tests := []struct {
		name          string
		externalEnv   string
		expectedData  ExternalData
		expectedError bool
	}{
		{
			name:        "Empty external data",
			externalEnv: "",
			expectedData: ExternalData{
				Init:          false,
				ContainerName: "",
				PodUID:        "",
			},
			expectedError: false,
		},
		{
			name:        "Valid external data with Init",
			externalEnv: "it-true,cn-container-name,pu-12345678-90ab-cdef-1234-567890abcdef",
			expectedData: ExternalData{
				Init:          true,
				ContainerName: "container-name",
				PodUID:        "12345678-90ab-cdef-1234-567890abcdef",
			},
			expectedError: false,
		},
		{
			name:          "Invalid Init value",
			externalEnv:   "it-invalid,cn-container-name,pu-12345678-90ab-cdef-1234-567890abcdef",
			expectedData:  ExternalData{},
			expectedError: true,
		},
		{
			name:        "Partial external data",
			externalEnv: "cn-container-name",
			expectedData: ExternalData{
				Init:          false,
				ContainerName: "container-name",
				PodUID:        "",
			},
			expectedError: false,
		},
		{
			name:        "Unrecognized prefix",
			externalEnv: "unknown-prefix",
			expectedData: ExternalData{
				Init:          false,
				ContainerName: "",
				PodUID:        "",
			},
			expectedError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := ParseExternalData(tc.externalEnv)

			if tc.expectedError {
				assert.Error(t, err)
			} else {
				assert.Equal(t, tc.expectedData, result)
			}
		})
	}
}
