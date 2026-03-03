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

func Test_GetProfileMap(t *testing.T) {
	t.Cleanup(ResetProfilesPath)
	SetConfdPathAndCleanProfiles()

	tests := []struct {
		name           string
		profileFolder  string
		expected       Map
		expectedErrMsg string
	}{
		{
			name:          "default profiles successful",
			profileFolder: "default_profiles",
			expected: Map{
				"p1": &NCMProfile{
					BaseProfile: BaseProfile{
						Name: "p1",
					},
					Commands: map[CommandType]*Commands{
						Running: {
							CommandType: Running,
							Values:      []string{"show run"},
						},
						Startup: {
							CommandType: Startup,
							Values:      []string{"show start"},
						},
						Version: {
							CommandType: Version,
							Values:      []string{"show ver"},
						},
					},
				},
				"p2": &NCMProfile{
					BaseProfile: BaseProfile{
						Name: "p2",
					},
					Commands: map[CommandType]*Commands{
						Running: runningCommandsWithCompiledRegex,
						Startup: {
							CommandType: Startup,
							Values:      []string{"show startup-config"},
						},
						Version: {
							CommandType: Version,
							Values:      []string{"show version"},
						},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := GetProfileMap(tt.profileFolder)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expected, actual)
		})
	}
}

func Test_GetCommandValues(t *testing.T) {
	np := &NCMProfile{
		BaseProfile: BaseProfile{
			Name: "test-profile",
		},
		Commands: map[CommandType]*Commands{
			Running: {
				CommandType: Running,
				Values:      []string{"show running-config"}},
		},
	}
	tests := []struct {
		name           string
		ncmProfile     *NCMProfile
		command        CommandType
		expectedOutput []string
		expectedErrMsg string
	}{
		{
			name:           "get running successful",
			ncmProfile:     np,
			command:        Running,
			expectedOutput: []string{"show running-config"},
		},
		{
			name:           "get startup failed",
			ncmProfile:     np,
			command:        Startup,
			expectedOutput: nil,
			expectedErrMsg: "could not find values for the command from the profile test-profile: startup",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmds, err := tt.ncmProfile.GetCommandValues(tt.command)
			if tt.expectedErrMsg != "" {
				assert.EqualError(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expectedOutput, cmds)
		})
	}
}
