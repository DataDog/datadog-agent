// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package channel

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
)

func TestComputeServiceName(t *testing.T) {
	assert.Equal(t, "agent", computeServiceName(nil, "toto"))
	lambdaConfig := &config.Lambda{}
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "my-service-name"))
	assert.Equal(t, "my-service-name", computeServiceName(lambdaConfig, "MY-SERVICE-NAME"))
	assert.Equal(t, "", computeServiceName(lambdaConfig, ""))
}

func TestComputeServiceNameFromCloudRunRevision(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("K_SERVICE", "superService")
	assert.Equal(t, "service-value", computeServiceName(nil, "service-value"))
	assert.Equal(t, "superservice", computeServiceName(nil, ""))
}

func TestNotServerlessModeKVersionUndefined(t *testing.T) {
	os.Setenv("K_SERVICE", "superService")
	defer os.Unsetenv("K_SERVICE")
	assert.False(t, isServerlessOrigin(nil))
}

func TestNotServerlessModeKServiceUndefined(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	assert.False(t, isServerlessOrigin(nil))
}

func TestServerlessModeCloudRun(t *testing.T) {
	os.Setenv("K_REVISION", "version-abc")
	defer os.Unsetenv("K_REVISION")
	os.Setenv("K_SERVICE", "superService")
	defer os.Unsetenv("K_SERVICE")
	assert.True(t, isServerlessOrigin(nil))
}

func TestServerlessModeLambda(t *testing.T) {
	lambdaConfig := &config.Lambda{}
	assert.True(t, isServerlessOrigin(lambdaConfig))
}
