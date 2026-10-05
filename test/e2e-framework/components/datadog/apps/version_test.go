// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package apps

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
)

type fakeEnv struct {
	config.Env

	internalRegistry string
}

func (e *fakeEnv) InternalRegistry() string {
	return e.internalRegistry
}

func TestImage(t *testing.T) {
	tests := []struct {
		name             string
		internalRegistry string
		repo             string
		want             string
	}{
		{"aws", "669783387624.dkr.ecr.us-east-1.amazonaws.com", "apps-dogstatsd", "669783387624.dkr.ecr.us-east-1.amazonaws.com/apps-dogstatsd:v0.0.9"},
		{"gcp", "us-central1-docker.pkg.dev/datadog-agent-qa/agent-qa", "apps-dogstatsd", "us-central1-docker.pkg.dev/datadog-agent-qa/agent-qa/apps-dogstatsd:v0.0.9"},
		{"azure", "agentqa.azurecr.io", "apps-dogstatsd", "agentqa.azurecr.io/apps-dogstatsd:v0.0.9"},
		{"local", "none", "apps-dogstatsd", "ghcr.io/datadog/apps-dogstatsd:v0.0.9"},
		{"no registry", "", "apps-dogstatsd", "ghcr.io/datadog/apps-dogstatsd:v0.0.9"},
		{"redis", "669783387624.dkr.ecr.us-east-1.amazonaws.com", "redis", "669783387624.dkr.ecr.us-east-1.amazonaws.com/redis:v0.0.9"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fakeEnv{internalRegistry: tt.internalRegistry}
			if got := Image(e, tt.repo); got != tt.want {
				t.Errorf("Image(%q, %q) = %q, want %q", tt.internalRegistry, tt.repo, got, tt.want)
			}
		})
	}
}
