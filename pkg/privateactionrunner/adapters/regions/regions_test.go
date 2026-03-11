// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package regions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetRegionFromDDSite(t *testing.T) {
	tests := []struct {
		name     string
		ddSite   string
		expected string
	}{
		{
			name:     "EU site",
			ddSite:   "datadoghq.eu",
			expected: "eu1",
		},
		{
			name:     "US1 site",
			ddSite:   "us1.datadoghq.com",
			expected: "us1",
		},
		{
			name:     "US3 site",
			ddSite:   "us3.datadoghq.com",
			expected: "us3",
		},
		{
			name:     "US5 site",
			ddSite:   "us5.datadoghq.com",
			expected: "us5",
		},
		{
			name:     "AP1 site",
			ddSite:   "ap1.datadoghq.com",
			expected: "ap1",
		},
		{
			name:     "default to US1 for unknown",
			ddSite:   "unknown.example.com",
			expected: "us1",
		},
		{
			name:     "default to US1 for empty",
			ddSite:   "",
			expected: "us1",
		},
		{
			name:     "default to US1 for datadoghq.com without prefix",
			ddSite:   "datadoghq.com",
			expected: "us1",
		},
		{
			name:     "gov site",
			ddSite:   "ddog-gov.datadoghq.com",
			expected: "ddog-gov",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GetRegionFromDDSite(tt.ddSite)
			assert.Equal(t, tt.expected, result)
		})
	}
}
