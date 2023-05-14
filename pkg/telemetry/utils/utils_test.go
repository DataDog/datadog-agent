// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestIsCheckEnabled(t *testing.T) {
	assert := assert.New(t)

	mockConfig := config.Mock(t)
	mockConfig.Set("telemetry.enabled", false)

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"*"})

	assert.True(IsCheckEnabled("cpu"))
	assert.True(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"cpu"})

	assert.True(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", false)
	mockConfig.Set("telemetry.checks", []string{"cpu"})

	assert.False(IsCheckEnabled("cpu"))
	assert.False(IsCheckEnabled("disk"))

	mockConfig.Set("telemetry.enabled", true)
	mockConfig.Set("telemetry.checks", []string{"cpu", "disk"})

	assert.True(IsCheckEnabled("cpu"))
	assert.True(IsCheckEnabled("disk"))
}
