// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test && ncm

package profile

import (
	"path/filepath"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func Test_GetProfileMap(t *testing.T) {
	mockConfig := configmock.New(t)
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	mockConfig.SetWithoutSource("confd_path", defaultTestConfdPath)

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
				"_base": &NCMProfile{
					BaseProfile: BaseProfile{
						Name: "_base",
					},
					Commands: map[CommandType][]string{},
				},
				"p1": &NCMProfile{
					BaseProfile: BaseProfile{
						Name: "p1",
					},
					Commands: map[CommandType][]string{
						Running: {"show run"},
						Startup: {"show start"},
						Version: {"show ver"},
					},
				},
				"p2": &NCMProfile{
					BaseProfile: BaseProfile{
						Name: "p2",
					},
					Commands: map[CommandType][]string{
						Running: {"show running-config"},
						Startup: {"show startup-config"},
						Version: {"show version"},
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
		Commands: map[CommandType][]string{
			Running: {"show running-config"},
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

func Test_ParseProfileFromFile(t *testing.T) {
	mockConfig := configmock.New(t)
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	mockConfig.SetWithoutSource("confd_path", defaultTestConfdPath)

	absPath, _ := filepath.Abs(filepath.Join(defaultTestConfdPath, "networkconfigmanagement.d", "default_profiles", "p2.yaml"))
	tests := []struct {
		name            string
		definitionType  Definition[any]
		profileFile     string
		expectedProfile *NCMProfileRaw
		expectedErrMsg  string
	}{
		{
			name:        "read NCM yaml profile successful",
			profileFile: absPath,
			expectedProfile: &NCMProfileRaw{
				Commands: []Commands{
					{CommandType: Running, Values: []string{"show running-config"}},
					{CommandType: Startup, Values: []string{"show startup-config"}},
					{CommandType: Version, Values: []string{"show version"}},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceProfile, err := ParseProfileFromFile[*NCMProfileRaw](tt.profileFile)
			if tt.expectedErrMsg != "" {
				assert.ErrorContains(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expectedProfile, deviceProfile)
		})
	}
}

func Test_ParseNCMProfileFromFile(t *testing.T) {
	SetConfdPathAndCleanProfiles()
	basePath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d", "networkconfigmanagement.d", "default_profiles"))
	p1 := filepath.Join(basePath, "p1.json")
	p2 := filepath.Join(basePath, "p2.yaml")

	tests := []struct {
		name                  string
		profileFile           string
		expectedDeviceProfile *NCMProfile
		expectedErrMsg        string
	}{
		{
			name:        "read NCM json profile successful",
			profileFile: p1,
			expectedDeviceProfile: &NCMProfile{
				Commands: map[CommandType][]string{
					Running: {"show run"},
					Startup: {"show start"},
					Version: {"show ver"},
				},
			},
		},
		{
			name:        "read NCM YAML profile successful",
			profileFile: p2,
			expectedDeviceProfile: &NCMProfile{
				Commands: map[CommandType][]string{
					Running: {"show running-config"},
					Startup: {"show startup-config"},
					Version: {"show version"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deviceProfile, err := ParseNCMProfileFromFile(tt.profileFile)
			if tt.expectedErrMsg != "" {
				assert.ErrorContains(t, err, tt.expectedErrMsg)
			}
			assert.Equal(t, tt.expectedDeviceProfile, deviceProfile)
		})
	}
}

func Test_resolveNCMProfileDefinitionPath(t *testing.T) {
	mockConfig := configmock.New(t)
	defaultTestConfdPath, _ := filepath.Abs(filepath.Join("..", "test", "conf.d"))
	mockConfig.SetWithoutSource("confd_path", defaultTestConfdPath)

	absPath, _ := filepath.Abs(filepath.Join("tmp", "myfile.yaml"))
	tests := []struct {
		name               string
		definitionFilePath string
		expectedPath       string
	}{
		{
			name:               "abs path",
			definitionFilePath: absPath,
			expectedPath:       absPath,
		},
		{
			name:               "relative path with default profile",
			definitionFilePath: "p2.yaml",
			expectedPath:       filepath.Join(mockConfig.Get("confd_path").(string), "networkconfigmanagement.d", "default_profiles", "p2.yaml"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := resolveNCMProfileDefinitionPath(tt.definitionFilePath)
			assert.Equal(t, tt.expectedPath, path)
		})
	}
}
