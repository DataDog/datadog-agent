// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package trace

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
)

// Test that tracerPayloadModifier implements TracerPayloadModifier interface
var _ agent.TracerPayloadModifier = (*tracerPayloadModifier)(nil)

func TestTracerPayloadModifier_Modify(t *testing.T) {
	tests := []struct {
		name                   string
		functionTags           string
		inputTags              map[string]string
		expectedTags           map[string]string
		shouldHaveFunctionTags bool
	}{
		{
			name:                   "empty function tags",
			functionTags:           "",
			inputTags:              map[string]string{"existing": "tag"},
			expectedTags:           map[string]string{"existing": "tag"},
			shouldHaveFunctionTags: false,
		},
		{
			name:                   "nil tags and empty function tags - should stay nil",
			functionTags:           "",
			inputTags:              nil,
			expectedTags:           nil,
			shouldHaveFunctionTags: false,
		},
		{
			name:         "valid function tags",
			functionTags: "env:production,service:my-service,version:1.0",
			inputTags:    map[string]string{"existing": "tag"},
			expectedTags: map[string]string{
				"existing":      "tag",
				tagFunctionTags: "env:production,service:my-service,version:1.0",
			},
			shouldHaveFunctionTags: true,
		},
		{
			name:         "replaces existing function tags",
			functionTags: "env:staging,service:new-service",
			inputTags: map[string]string{
				tagFunctionTags: "env:production,service:old-service",
				"other":         "tag",
			},
			expectedTags: map[string]string{
				tagFunctionTags: "env:staging,service:new-service",
				"other":         "tag",
			},
			shouldHaveFunctionTags: true,
		},
		{
			name:         "replaces existing empty function tags",
			functionTags: "env:test,service:test-service",
			inputTags: map[string]string{
				tagFunctionTags: "",
				"other":         "tag",
			},
			expectedTags: map[string]string{
				tagFunctionTags: "env:test,service:test-service",
				"other":         "tag",
			},
			shouldHaveFunctionTags: true,
		},
		{
			name:         "identical function tags - idempotent",
			functionTags: "env:production,service:same-service",
			inputTags: map[string]string{
				tagFunctionTags: "env:production,service:same-service",
				"other":         "tag",
			},
			expectedTags: map[string]string{
				tagFunctionTags: "env:production,service:same-service",
				"other":         "tag",
			},
			shouldHaveFunctionTags: true,
		},
		{
			name:         "nil tags map",
			functionTags: "env:test,service:test-service",
			inputTags:    nil,
			expectedTags: map[string]string{
				tagFunctionTags: "env:test,service:test-service",
			},
			shouldHaveFunctionTags: true,
		},
		{
			name:         "empty tags map",
			functionTags: "env:dev,service:dev-service",
			inputTags:    make(map[string]string),
			expectedTags: map[string]string{
				tagFunctionTags: "env:dev,service:dev-service",
			},
			shouldHaveFunctionTags: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			modifier := &tracerPayloadModifier{
				functionTags: tt.functionTags,
			}

			payload := &pb.TracerPayload{
				Tags: tt.inputTags,
			}

			modifier.Modify(payload)

			if tt.shouldHaveFunctionTags {
				assert.Contains(t, payload.Tags, tagFunctionTags)
				assert.Equal(t, tt.functionTags, payload.Tags[tagFunctionTags])
			} else {
				assert.NotContains(t, payload.Tags, tagFunctionTags)
			}

			assert.Equal(t, tt.expectedTags, payload.Tags)
		})
	}
}
