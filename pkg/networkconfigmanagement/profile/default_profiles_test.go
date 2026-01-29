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
			fixture:                   loadFixture("pan-os", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "AOSW",
			profile:                   DefaultProfile("aosw"),
			fixture:                   loadFixture("aosw", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "NXOS",
			profile: DefaultProfile("nxos"),
			fixture: loadFixture("nxos", Running),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1767709263,
			},
		},
		{
			name:                      "TMOS",
			profile:                   DefaultProfile("tmos"),
			fixture:                   loadFixture("tmos", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "AOSCX",
			profile:                   DefaultProfile("aoscx"),
			fixture:                   loadFixture("aoscx", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "EOS",
			profile:                   DefaultProfile("eos"),
			fixture:                   loadFixture("eos", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "fortios",
			profile:                   DefaultProfile("fortios"),
			fixture:                   loadFixture("fortios", Running),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "DellOS10",
			profile: DefaultProfile("dellos10"),
			fixture: loadFixture("dellos10", Running),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1491873902,
			},
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

func Test_DefaultProfiles_Startup(t *testing.T) {
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
			fixture: loadFixture("cisco-ios", Startup),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp:  1765307830,
				ConfigSize: 3163,
			},
		},
		{
			name:    "NXOS",
			profile: DefaultProfile("nxos"),
			fixture: loadFixture("nxos", Startup),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1767899167,
			},
		},
		{
			name:                      "AOSCX",
			profile:                   DefaultProfile("aoscx"),
			fixture:                   loadFixture("aoscx", Startup),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "EOS",
			profile: DefaultProfile("eos"),
			fixture: loadFixture("eos", Startup),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1392798871,
				Author:    "admin",
			},
		},
		{
			name:                      "dellos10",
			profile:                   DefaultProfile("dellos10"),
			fixture:                   loadFixture("dellos10", Startup),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.profile.initializeScrubbers()
			actualOutput, actualExtractedMetadata, err := tt.profile.ProcessCommandOutput(Startup, tt.fixture.Initial)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.fixture.Expected, actualOutput)
			assert.Equal(t, tt.expectedExtractedMetadata, actualExtractedMetadata)
		})
	}
}
