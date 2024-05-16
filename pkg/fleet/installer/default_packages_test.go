// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installer

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultPackages(t *testing.T) {
	// Test cases
	tests := []struct {
		name           string
		envVariables   map[string]string
		expectedResult map[string]string
	}{
		{
			name: "Empty packages",
			envVariables: map[string]string{
				"DD_INSTALLER_PACKAGES":          "",
				"DD_APM_INSTRUMENTATION_ENABLED": "",
			},
			expectedResult: map[string]string{},
		},
		{
			name: "Forced packages",
			envVariables: map[string]string{
				"DD_INSTALLER_PACKAGES": "package1:1.0.0,package2:2.0.0,package3",
			},
			expectedResult: map[string]string{
				"package1": "1.0.0",
				"package2": "2.0.0",
				"package3": "latest",
			},
		},
		{
			name: "APM instrumentation enabled",
			envVariables: map[string]string{
				"DD_APM_INSTRUMENTATION_ENABLED": "all",
			},
			expectedResult: map[string]string{
				"datadog-apm-inject": "latest",
			},
		},
		{
			name: "Forced packages override APM instrumentation",
			envVariables: map[string]string{
				"DD_INSTALLER_PACKAGES":          "datadog-apm-inject:1.0.0",
				"DD_APM_INSTRUMENTATION_ENABLED": "all",
			},
			expectedResult: map[string]string{
				"datadog-apm-inject": "1.0.0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for key, value := range tt.envVariables {
				os.Setenv(key, value)
				defer os.Unsetenv(key)
			}
			result := DefaultPackages()
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}
