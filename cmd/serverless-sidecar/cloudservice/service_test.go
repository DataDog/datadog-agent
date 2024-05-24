// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cloudservice

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetCloudServiceType(t *testing.T) {
	assert.Equal(t, "local", GetCloudServiceType().GetOrigin())

	t.Setenv(ContainerAppNameEnvVar, "test-name")
	assert.Equal(t, "containerapp", GetCloudServiceType().GetOrigin())

	t.Setenv(ServiceNameEnvVar, "test-name")
	assert.Equal(t, "cloudrun", GetCloudServiceType().GetOrigin())

	os.Unsetenv(ContainerAppNameEnvVar)
	os.Unsetenv(ServiceNameEnvVar)
	t.Setenv(RunZip, "false")
	assert.Equal(t, "appservice", GetCloudServiceType().GetOrigin())
}
