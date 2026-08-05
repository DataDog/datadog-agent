// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package payloadmodifierimpl

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
)

func TestNewComponent_AzureAppServices(t *testing.T) {
	tests := []struct {
		name                    string
		azureAppServicesEnabled bool
		tags                    []string
		extraTags               []string
		dogstatsdTags           []string
		expectedFunctionTags    string
		expectModifier          bool
	}{
		{
			name:                    "Azure App Services enabled with tags and extra_tags",
			azureAppServicesEnabled: true,
			tags:                    []string{"env:production", "service:my-service"},
			extraTags:               []string{"team:backend", "region:us-east-1"},
			dogstatsdTags:           []string{"dogstatsd:should-not-appear"},
			expectedFunctionTags:    "env:production,service:my-service,team:backend,region:us-east-1",
			expectModifier:          true,
		},
		{
			name:                    "Azure App Services enabled with only tags",
			azureAppServicesEnabled: true,
			tags:                    []string{"env:staging"},
			extraTags:               []string{},
			dogstatsdTags:           []string{"dogstatsd:should-not-appear"},
			expectedFunctionTags:    "env:staging",
			expectModifier:          true,
		},
		{
			name:                    "Azure App Services enabled with only extra_tags",
			azureAppServicesEnabled: true,
			tags:                    []string{},
			extraTags:               []string{"team:frontend"},
			dogstatsdTags:           []string{"dogstatsd:should-not-appear"},
			expectedFunctionTags:    "team:frontend",
			expectModifier:          true,
		},
		{
			name:                    "Azure App Services enabled with no tags",
			azureAppServicesEnabled: true,
			tags:                    []string{},
			extraTags:               []string{},
			dogstatsdTags:           []string{"dogstatsd:should-not-appear"},
			expectedFunctionTags:    "",
			expectModifier:          true,
		},
		{
			name:                    "Azure App Services disabled",
			azureAppServicesEnabled: false,
			tags:                    []string{"env:production"},
			extraTags:               []string{"team:backend"},
			dogstatsdTags:           []string{"dogstatsd:should-not-appear"},
			expectedFunctionTags:    "",
			expectModifier:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set up environment variable for Azure App Services
			if tt.azureAppServicesEnabled {
				os.Setenv(serverlessenv.AzureAppServicesEnvVar, "1")
			} else {
				os.Unsetenv(serverlessenv.AzureAppServicesEnvVar)
			}
			defer os.Unsetenv(serverlessenv.AzureAppServicesEnvVar)

			// Create mock config
			config := configmock.New(t)
			config.SetInTest("tags", tt.tags)
			config.SetInTest("extra_tags", tt.extraTags)
			config.SetInTest("dogstatsd_tags", tt.dogstatsdTags)

			// Create component
			deps := Dependencies{
				Config: config,
			}
			provides := NewComponent(deps)

			// Verify component was created
			require.NotNil(t, provides.Comp)

			comp := provides.Comp.(*component)

			if tt.expectModifier {
				// Verify modifier was created
				require.NotNil(t, comp.modifier, "expected modifier to be created")

				// Test that the modifier applies the expected tags
				payload := &pb.TracerPayload{
					Tags: map[string]string{
						"existing": "tag",
					},
				}

				comp.Modify(payload)

				if tt.expectedFunctionTags != "" {
					// Verify function tags were added
					assert.Contains(t, payload.Tags, "_dd.tags.function")
					functionTags := payload.Tags["_dd.tags.function"]
					assert.Equal(t, tt.expectedFunctionTags, functionTags)
					// Verify dogstatsd tags are excluded
					assert.NotContains(t, functionTags, "dogstatsd")
					assert.NotContains(t, functionTags, "should-not-appear")
				}

				// Verify existing tags are preserved
				assert.Equal(t, "tag", payload.Tags["existing"])
			} else {
				// Verify modifier was not created
				assert.Nil(t, comp.modifier, "expected modifier to be nil")

				// Test that Modify is a no-op
				payload := &pb.TracerPayload{
					Tags: map[string]string{
						"existing": "tag",
					},
				}

				comp.Modify(payload)

				// Payload should be unchanged
				assert.Equal(t, map[string]string{"existing": "tag"}, payload.Tags)
			}
		})
	}
}
