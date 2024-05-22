// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
package env

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected *Env
	}{
		{
			name: "Empty environment variables",
			envVars: map[string]string{
				envAPIKey:         "",
				envSite:           "",
				envRegistry:       "",
				envRegistryAuth:   "",
				envDefaultVersion: "",
			},
			expected: &Env{
				APIKey:                          "",
				Site:                            "datadoghq.com",
				RegistryOverride:                "",
				RegistryAuthOverride:            "",
				RegistryOverrideByImage:         map[string]string{},
				RegistryAuthOverrideByImage:     map[string]string{},
				DefaultVersionOverrideByPackage: map[string]string{},
			},
		},
		{
			name: "All environment variables set",
			envVars: map[string]string{
				envAPIKey:                              "123456",
				envSite:                                "datadoghq.eu",
				envRegistry:                            "registry.example.com",
				envRegistryAuth:                        "auth",
				envRegistry + "_IMAGE":                 "another.registry.example.com",
				envRegistry + "_ANOTHER_IMAGE":         "yet.another.registry.example.com",
				envRegistryAuth + "_IMAGE":             "another.auth",
				envRegistryAuth + "_ANOTHER_IMAGE":     "yet.another.auth",
				envDefaultVersion + "_PACKAGE":         "1.2.3",
				envDefaultVersion + "_ANOTHER_PACKAGE": "4.5.6",
			},
			expected: &Env{
				APIKey:               "123456",
				Site:                 "datadoghq.eu",
				RegistryOverride:     "registry.example.com",
				RegistryAuthOverride: "auth",
				RegistryOverrideByImage: map[string]string{
					"image":         "another.registry.example.com",
					"another-image": "yet.another.registry.example.com",
				},
				RegistryAuthOverrideByImage: map[string]string{
					"image":         "another.auth",
					"another-image": "yet.another.auth",
				},
				DefaultVersionOverrideByPackage: map[string]string{
					"package":         "1.2.3",
					"another-package": "4.5.6",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVars {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			result := FromEnv()
			assert.Equal(t, tt.expected, result)
		})
	}
}
