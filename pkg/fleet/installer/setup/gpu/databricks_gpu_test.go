// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/common"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/setup/config"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

func TestCTracerVersion(t *testing.T) {
	tests := []struct {
		name     string
		env      string
		expected string
	}{
		{name: "unset uses latest major", env: "", expected: "0"},
		{name: "full version gets revision", env: "0.40.0", expected: "0.40.0-1"},
		{name: "version with revision kept", env: "0.40.0-1", expected: "0.40.0-1"},
		{name: "minor tag kept", env: "0.40", expected: "0.40"},
		{name: "major tag kept", env: "1", expected: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(envCTracerVersion, tt.env)
			assert.Equal(t, tt.expected, cTracerVersion())
		})
	}
}

func TestSetupGPUCheck(t *testing.T) {
	span, _ := telemetry.StartSpanFromContext(context.Background(), "test")
	s := &common.Setup{Span: span, Out: &common.Output{}}

	setupGPUCheck(s)

	assert.Equal(t, config.BoolToPtr(true), s.Config.DatadogYAML.GPUCheck.Enabled)
	assert.True(t, s.DelayedAgentRestartConfig.Scheduled)
	assert.Equal(t, gpuRestartDelay, s.DelayedAgentRestartConfig.Delay)
	assert.Equal(t, gpuRestartLogFile, s.DelayedAgentRestartConfig.LogFile)
}

func TestAgentVersion(t *testing.T) {
	tests := []struct {
		name     string
		env      *env.Env
		expected string
	}{
		{name: "no env uses latest Agent 7", env: nil, expected: "7"},
		{name: "unset uses latest Agent 7", env: &env.Env{}, expected: "7"},
		{name: "minor version pin", env: &env.Env{AgentMinorVersion: "86.0"}, expected: "7.86.0-1"},
		{name: "minor version with revision", env: &env.Env{AgentMinorVersion: "86.0-1"}, expected: "7.86.0-1"},
		{name: "major and minor", env: &env.Env{AgentMajorVersion: "7", AgentMinorVersion: "86"}, expected: "7.86"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, agentVersion(&common.Setup{Env: tt.env}))
		})
	}
}
