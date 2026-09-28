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
	config.Env // embedded to satisfy the interface; only InternalRegistry is used

	internalRegistry string
}

func (e *fakeEnv) InternalRegistry() string {
	return e.internalRegistry
}

func TestImagePath(t *testing.T) {
	tests := []struct {
		name             string
		internalRegistry string
		repo             string
		want             string
	}{
		{
			name:             "AWS ECR internal registry",
			internalRegistry: "669783387624.dkr.ecr.us-east-1.amazonaws.com",
			repo:             "apps-dogstatsd",
			want:             "669783387624.dkr.ecr.us-east-1.amazonaws.com/apps-dogstatsd",
		},
		{
			name:             "GCP Artifact Registry internal registry",
			internalRegistry: "us-central1-docker.pkg.dev/datadog-agent-qa/agent-qa",
			repo:             "apps-dogstatsd",
			want:             "us-central1-docker.pkg.dev/datadog-agent-qa/agent-qa/apps-dogstatsd",
		},
		{
			name:             "Azure ACR internal registry",
			internalRegistry: "agentqa.azurecr.io",
			repo:             "apps-dogstatsd",
			want:             "agentqa.azurecr.io/apps-dogstatsd",
		},
		{
			name:             "local environment (\"none\" placeholder)",
			internalRegistry: "none",
			repo:             "apps-dogstatsd",
			want:             "ghcr.io/datadog/apps-dogstatsd",
		},
		{
			name:             "no internal registry",
			internalRegistry: "",
			repo:             "apps-dogstatsd",
			want:             "ghcr.io/datadog/apps-dogstatsd",
		},
		{
			name:             "redis server app has no apps- prefix",
			internalRegistry: "669783387624.dkr.ecr.us-east-1.amazonaws.com",
			repo:             "redis",
			want:             "669783387624.dkr.ecr.us-east-1.amazonaws.com/redis",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &fakeEnv{internalRegistry: tt.internalRegistry}
			if got := ImagePath(e, tt.repo); got != tt.want {
				t.Errorf("ImagePath(%q, %q) = %q, want %q", tt.internalRegistry, tt.repo, got, tt.want)
			}
			wantImage := tt.want + ":" + Version
			if got := Image(e, tt.repo); got != wantImage {
				t.Errorf("Image(%q, %q) = %q, want %q", tt.internalRegistry, tt.repo, got, wantImage)
			}
		})
	}
}
