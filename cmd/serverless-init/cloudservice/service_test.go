// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	serverlessenv "github.com/DataDog/datadog-agent/pkg/serverless/env"
	serverlessMetrics "github.com/DataDog/datadog-agent/pkg/serverless/metrics"
)

func TestGetCloudServiceType(t *testing.T) {
	assert.Equal(t, "local", GetCloudServiceType().GetOrigin())

	t.Setenv(ContainerAppNameEnvVar, "test-name")
	assert.Equal(t, "containerapp", GetCloudServiceType().GetOrigin())

	t.Setenv(ServiceNameEnvVar, "test-name")
	assert.Equal(t, "cloudrun", GetCloudServiceType().GetOrigin())

	os.Unsetenv(ContainerAppNameEnvVar)
	os.Unsetenv(ServiceNameEnvVar)
	t.Setenv(WebsiteStack, "false")
	assert.Equal(t, "appservice", GetCloudServiceType().GetOrigin())
}

func TestGetCloudServiceTypeForCloudRunJob(t *testing.T) {
	t.Setenv("CLOUD_RUN_JOB", "test-job")
	cloudService := GetCloudServiceType()
	assert.Equal(t, "cloudrunjobs", cloudService.GetOrigin())

	// Verify it's the correct type
	_, ok := cloudService.(*CloudRunJobs)
	assert.True(t, ok)
}

func TestLocalServiceShutdownEmitsMetrics(t *testing.T) {
	skipOnWindows(t)
	demux := createDemultiplexer(t)
	agent := &serverlessMetrics.ServerlessMetricAgent{Demux: demux}

	service := &LocalService{}
	service.Shutdown(agent, true, nil)

	generatedMetrics, timedMetrics := demux.WaitForSamples(100 * time.Millisecond)
	assert.Empty(t, timedMetrics)
	assert.Len(t, generatedMetrics, 1)

	foundShutdown := false
	for _, sample := range generatedMetrics {
		if sample.Name == localServiceShutdownMetricName {
			foundShutdown = true
		}
	}
	assert.True(t, foundShutdown, "shutdown metric not emitted")
}

func TestLocalServiceShutdownNilMetricAgent(t *testing.T) {
	service := &LocalService{}
	require.NotPanics(t, func() {
		service.Shutdown(nil, true, nil)
	})
}

func TestGetCloudServiceTypeMicroVM(t *testing.T) {
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image")
	svc := GetCloudServiceType()
	_, ok := svc.(*MicroVM)
	assert.True(t, ok, "expected MicroVM CloudService")
}

func TestGetCloudServiceTypeMicroVMTakesPriorityOverCloudRun(t *testing.T) {
	// MicroVM is checked first — both would never be set in practice,
	// but the ordering must be explicit.
	t.Setenv(ServiceNameEnvVar, "my-service")
	t.Setenv(serverlessenv.MicroVMImageARNEnvVar, "arn:aws:lambda:us-east-1:123456789012:microvm-image:my-image")
	svc := GetCloudServiceType()
	_, ok := svc.(*MicroVM)
	assert.True(t, ok, "MicroVM should take priority over CloudRun")
}
