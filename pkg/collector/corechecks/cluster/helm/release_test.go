// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetConfigValue(t *testing.T) {
	testRelease := release{
		Name: "test release",
		Chart: &chart{
			Values: map[string]interface{}{
				"agents": map[string]interface{}{
					"image": map[string]string{
						"tag":        "7.39.0",
						"repository": "datadog/agent", // not overridden
					},
				},
			},
		},
		Config: map[string]interface{}{
			"agents": map[string]interface{}{
				"image": map[string]string{
					"tag": "latest", // overrides default value
				},
			},
		},
	}

	tests := []struct {
		name            string
		rel             release
		dotSeparatedKey string
		expectedValue   string
		expectsError    bool
	}{
		{
			name:            "config param has been set",
			rel:             testRelease,
			dotSeparatedKey: "agents.image.tag",
			expectedValue:   "latest",
		},
		{
			name:            "config param has a default value in the chart",
			rel:             testRelease,
			dotSeparatedKey: "agents.image.repository",
			expectedValue:   "datadog/agent",
		},
		{
			name:            "config param not set",
			rel:             testRelease,
			dotSeparatedKey: "agents.abc",
			expectsError:    true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := test.rel.getConfigValue(test.dotSeparatedKey)

			if test.expectsError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, test.expectedValue, result)
			}
		})
	}
}
