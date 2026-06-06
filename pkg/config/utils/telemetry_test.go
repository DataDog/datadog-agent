// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestIsCheckTelemetryEnabled(t *testing.T) {
	assert := assert.New(t)

	mockConfig := configmock.New(t)
	mockConfig.SetInTest("agent_telemetry.enabled", false)
	mockConfig.SetInTest("telemetry.enabled", false)

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("telemetry.enabled", true)

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("telemetry.enabled", true)
	mockConfig.SetInTest("telemetry.checks", []string{"*"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.True(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("telemetry.enabled", true)
	mockConfig.SetInTest("telemetry.checks", []string{"cpu"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("telemetry.enabled", false)
	mockConfig.SetInTest("telemetry.checks", []string{"cpu"})

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("telemetry.enabled", true)
	mockConfig.SetInTest("telemetry.checks", []string{"cpu", "disk"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.True(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetInTest("agent_telemetry.enabled", true)
	mockConfig.SetInTest("site", "xx.ddog-gov.com")
	assert.False(IsAgentTelemetryEnabled(mockConfig))
}
