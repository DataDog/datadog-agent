// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestECRRepositoryName(t *testing.T) {
	tests := []struct {
		name     string
		image    string
		expected string
	}{
		{
			name:     "pipeline image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/agent-qa",
			expected: "agent-qa",
		},
		{
			// Pull-through cache repositories are nested, so the repository name keeps
			// every segment after the host.
			name:     "pull-through cache image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/ecr-public/datadog/agent",
			expected: "ecr-public/datadog/agent",
		},
		{
			name:     "dockerhub mirror image",
			image:    "669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/datadog/agent-dev",
			expected: "dockerhub/datadog/agent-dev",
		},
		{
			name:     "no host",
			image:    "agent",
			expected: "agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ecrRepositoryName(tt.image))
		})
	}
}
