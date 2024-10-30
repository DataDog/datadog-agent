// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

func TestIsCheckTelemetryEnabled(t *testing.T) {
	assert := assert.New(t)

	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("agent_telemetry.enabled", false)
	mockConfig.SetWithoutSource("telemetry.enabled", false)

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetWithoutSource("telemetry.enabled", true)

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetWithoutSource("telemetry.enabled", true)
	mockConfig.SetWithoutSource("telemetry.checks", []string{"*"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.True(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetWithoutSource("telemetry.enabled", true)
	mockConfig.SetWithoutSource("telemetry.checks", []string{"cpu"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetWithoutSource("telemetry.enabled", false)
	mockConfig.SetWithoutSource("telemetry.checks", []string{"cpu"})

	assert.False(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.False(IsCheckTelemetryEnabled("disk", mockConfig))

	mockConfig.SetWithoutSource("telemetry.enabled", true)
	mockConfig.SetWithoutSource("telemetry.checks", []string{"cpu", "disk"})

	assert.True(IsCheckTelemetryEnabled("cpu", mockConfig))
	assert.True(IsCheckTelemetryEnabled("disk", mockConfig))
}
