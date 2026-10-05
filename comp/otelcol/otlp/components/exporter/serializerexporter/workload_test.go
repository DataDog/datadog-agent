// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func clearWorkloadIdentityEnv(t *testing.T) {
	for _, envVar := range []string{
		ecsFargateEnvVar,
		awsExecutionEnvEnvVar,
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

func TestDetectWorkloadIdentity_Fargate(t *testing.T) {
	clearWorkloadIdentityEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"TaskARN":"arn:aws:ecs:us-east-1:123:task/cluster/abc"}`))
	}))
	defer server.Close()

	t.Setenv(ecsFargateEnvVar, "true")
	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	wi := detectWorkloadIdentity(context.Background(), zap.NewNop())

	assert.Equal(t, "arn:aws:ecs:us-east-1:123:task/cluster/abc", wi.fargateTaskARN)
	assert.Nil(t, wi.aca)
}

func TestDetectWorkloadIdentity_FargateViaAWSExecutionEnv(t *testing.T) {
	clearWorkloadIdentityEnv(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"TaskARN":"arn:aws:ecs:eu-west-1:456:task/cluster/def"}`))
	}))
	defer server.Close()

	t.Setenv(awsExecutionEnvEnvVar, "AWS_ECS_FARGATE")
	t.Setenv(ecsMetadataURIv4EnvVar, server.URL)

	wi := detectWorkloadIdentity(context.Background(), zap.NewNop())

	assert.Equal(t, "arn:aws:ecs:eu-west-1:456:task/cluster/def", wi.fargateTaskARN)
}

func TestDetectWorkloadIdentity_FargateFetchFailsFallsBackToEmpty(t *testing.T) {
	clearWorkloadIdentityEnv(t)

	t.Setenv(ecsFargateEnvVar, "true")
	// ecsMetadataURIv4EnvVar deliberately left unset, so the fetch fails.

	wi := detectWorkloadIdentity(context.Background(), zap.NewNop())

	assert.Empty(t, wi.fargateTaskARN)
	assert.Nil(t, wi.aca)
}

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
