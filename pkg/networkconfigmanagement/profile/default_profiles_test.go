// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_DefaultProfiles_Running(t *testing.T) {
	tests := []struct {
		name                      string
		profile                   *NCMProfile
		fixture                   Fixture
		expectedExtractedMetadata *ExtractedMetadata
		expectedErrMsg            string
	}{
		{
			name:    "Cisco IOS",
			profile: IOSProfile(),
			fixture: loadFixture("cisco-ios", Running),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp:  1760099696,
				ConfigSize: 3781,
			},
		},
		{
			name:    "JunOS",
			profile: JunOSProfile(),
			fixture: loadFixture("junos", Running),
			expectedExtractedMetadata: &ExtractedMetadata{
				ConfigSize: 0,
				Timestamp:  1730646727,
				Author:     "netops",
			},
		},
		{
			name:                      "PAN-OS",
			profile:                   DefaultProfile("pan-os"),
			fixture:                   loadFixture("pan-os"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "AOSW",
			profile:                   DefaultProfile("aosw"),
			fixture:                   loadFixture("aosw"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.profile.initializeScrubbers()
			actualOutput, actualExtractedMetadata, err := tt.profile.ProcessCommandOutput(Running, tt.fixture.Initial)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.fixture.Expected, actualOutput)
			assert.Equal(t, tt.expectedExtractedMetadata, actualExtractedMetadata)
		})
	}
}
