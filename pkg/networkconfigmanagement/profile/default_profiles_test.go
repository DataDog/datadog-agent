// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package profile

import (
	"testing"

	"github.com/google/go-cmp/cmp"
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
			name:                      "Cisco ASA",
			profile:                   DefaultProfile(t, ProfileCiscoASA),
			fixture:                   loadFixture(ProfileCiscoASA, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "Cisco IOS",
			profile: DefaultProfile(t, ProfileCiscoIOS),
			fixture: loadFixture(ProfileCiscoIOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp:  1760099696,
				ConfigSize: 3781,
			},
		},
		{
			name:    "JunOS",
			profile: DefaultProfile(t, ProfileJunos),
			fixture: loadFixture(ProfileJunos, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{
				ConfigSize: 0,
				Timestamp:  1730646727,
				Author:     "netops",
			},
		},
		{
			name:                      "PAN-OS",
			profile:                   DefaultProfile(t, ProfilePanOS),
			fixture:                   loadFixture(ProfilePanOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "AOSW",
			profile:                   DefaultProfile(t, ProfileAOSW),
			fixture:                   loadFixture(ProfileAOSW, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "NXOS",
			profile: DefaultProfile(t, ProfileNXOS),
			fixture: loadFixture(ProfileNXOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1767709263,
			},
		},
		{
			name:                      "TMOS",
			profile:                   DefaultProfile(t, ProfileTMOS),
			fixture:                   loadFixture(ProfileTMOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "AOSCX",
			profile:                   DefaultProfile(t, ProfileAOSCX),
			fixture:                   loadFixture(ProfileAOSCX, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "EOS",
			profile:                   DefaultProfile(t, ProfileEOS),
			fixture:                   loadFixture(ProfileEOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:                      "fortios",
			profile:                   DefaultProfile(t, ProfileFortiOS),
			fixture:                   loadFixture(ProfileFortiOS, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "DellOS10",
			profile: DefaultProfile(t, ProfileDellOS10),
			fixture: loadFixture(ProfileDellOS10, "running"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1491873902,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.profile.ProcessConfig(tt.fixture.Initial)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}

			// use cmp.Diff for a nicer output if the strings don't match, but still assert that they are equal
			assert.Empty(t, cmp.Diff(string(tt.fixture.Expected), string(result.Redacted)))
			assert.Equal(t, tt.expectedExtractedMetadata, result.Metadata)
		})
	}
}

func Test_TMOSGetRunningValidator(t *testing.T) {
	v := DefaultProfile(t, ProfileTMOS).Commands.GetRunning.Validator
	assert.NoError(t, v.Validate("#TMSH-VERSION: 17.1.3\n"))
	assert.NoError(t, v.Validate("sys global-settings {\n"))
	assert.NoError(t, v.Validate("ltm virtual /Common/x {\n"))
	assert.NoError(t, v.Validate("ltm pool /Common/pool_1 {\n"))
	assert.NoError(t, v.Validate("ltm node /Common/node_1 {\n"))
	assert.Error(t, v.Validate("not a tmos config header\n"))
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
			profile: DefaultProfile(t, ProfileCiscoIOS),
			fixture: loadFixture(ProfileCiscoIOS, "startup"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp:  1765307830,
				ConfigSize: 3163,
			},
		},
		{
			name:    "NXOS",
			profile: DefaultProfile(t, ProfileNXOS),
			fixture: loadFixture(ProfileNXOS, "startup"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1767899167,
			},
		},
		{
			name:                      "AOSCX",
			profile:                   DefaultProfile(t, ProfileAOSCX),
			fixture:                   loadFixture(ProfileAOSCX, "startup"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
		{
			name:    "EOS",
			profile: DefaultProfile(t, ProfileEOS),
			fixture: loadFixture(ProfileEOS, "startup"),
			expectedExtractedMetadata: &ExtractedMetadata{
				Timestamp: 1392798871,
				Author:    "admin",
			},
		},
		{
			name:                      "dellos10",
			profile:                   DefaultProfile(t, ProfileDellOS10),
			fixture:                   loadFixture(ProfileDellOS10, "startup"),
			expectedExtractedMetadata: &ExtractedMetadata{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.profile.ProcessConfig(tt.fixture.Initial)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}

			// use cmp.Diff for a nicer output if the strings don't match, but still assert that they are equal
			assert.Empty(t, cmp.Diff(string(tt.fixture.Expected), string(result.Redacted)))
			assert.Equal(t, tt.expectedExtractedMetadata, result.Metadata)
		})
	}
}
