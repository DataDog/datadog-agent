// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func clearWorkloadIdentityEnv(t *testing.T) {
	for _, envVar := range []string{
		"ECS_FARGATE",
		"AWS_EXECUTION_ENV",
		ecsMetadataURIv4EnvVar,
		containerAppNameEnvVar,
		containerAppReplicaNameEnvVar,
		azureSubscriptionIDEnvVar,
		azureResourceGroupEnvVar,
	} {
		prev, existed := os.LookupEnv(envVar)
		require.NoError(t, os.Unsetenv(envVar))
		t.Cleanup(func() {
			if existed {
				os.Setenv(envVar, prev)
			}
		})
	}
}

func TestDetectWorkloadIdentity_None(t *testing.T) {
	clearWorkloadIdentityEnv(t)

	wi := detectWorkloadIdentity(context.Background(), zap.NewNop())

	assert.Empty(t, wi.fargateTaskARN)
	assert.Nil(t, wi.aca)
}

// Note: fargate.GetOrchestrator() reads from pkg/config/env's package-level
// detected-features cache, whose detection is a no-op in unit test binaries
// (see env.DetectFeatures's detectionAlwaysDisabledInTests guard) -- so the
// ECS Fargate branch of detectWorkloadIdentity cannot be driven by env vars
// here. fetchECSTaskARN itself is covered directly in ecsfargate_test.go.

func TestDetectWorkloadIdentity_AzureContainerApps(t *testing.T) {
	clearWorkloadIdentityEnv(t)

	t.Setenv(containerAppNameEnvVar, "my-app")
	t.Setenv(containerAppReplicaNameEnvVar, "my-app-replica")
	t.Setenv(azureSubscriptionIDEnvVar, "sub-123")
	t.Setenv(azureResourceGroupEnvVar, "my-rg")

	wi := detectWorkloadIdentity(context.Background(), zap.NewNop())

	assert.Empty(t, wi.fargateTaskARN)
	require.NotNil(t, wi.aca)
}
