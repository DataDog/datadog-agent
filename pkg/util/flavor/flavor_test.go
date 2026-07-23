// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flavor

import (
	"fmt"
	"testing"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/stretchr/testify/assert"
)

// TestIotAgentNameFromFlavor verifies that GetFlavor returns IotAgent for an IoT agent binary
// even when iot_host is reset to false by a config re-initialization (regression introduced in 7.76.0).
// The agentFlavor package variable is authoritative and is not affected by InitConfigObjects.
func TestIotAgentNameFromFlavor(t *testing.T) {
	// configmock.New must be called first so it captures the original global config before
	// SetFlavor modifies it (SetFlavor writes iot_host=true to the global config).
	mockConfig := configmock.New(t)

	originalFlavor := agentFlavor
	t.Cleanup(func() { agentFlavor = originalFlavor })

	SetFlavor(IotAgent)

	// Simulate what happens when InitConfigObjects resets the config: iot_host becomes false.
	mockConfig.SetInTest("iot_host", false)

	assert.Equal(t, IotAgent, GetFlavor(), "IoT agent must report as iot_agent even when iot_host is false after config re-init")
}

// TestIotHostOverridePromotesDefaultAgent verifies that setting iot_host=true on a non-IoT agent
// still promotes it to report as iot_agent.
func TestIotHostOverridePromotesDefaultAgent(t *testing.T) {
	// configmock.New must be called before SetFlavor to correctly capture the original config.
	mockConfig := configmock.New(t)

	originalFlavor := agentFlavor
	t.Cleanup(func() { agentFlavor = originalFlavor })

	agentFlavor = DefaultAgent
	mockConfig.SetInTest("iot_host", true)

	assert.Equal(t, IotAgent, GetFlavor(), "iot_host=true must promote a default agent to report as iot_agent")
}

// TestInfrastructureModeIotPromotesToIotAgent verifies that setting
// infrastructure_mode=iot on a default agent binary makes GetFlavor return IotAgent.
func TestInfrastructureModeIotPromotesToIotAgent(t *testing.T) {
	mockConfig := configmock.New(t)

	originalFlavor := agentFlavor
	t.Cleanup(func() { agentFlavor = originalFlavor })

	agentFlavor = DefaultAgent
	mockConfig.SetInTest("infrastructure_mode", "iot")

	assert.Equal(t, IotAgent, GetFlavor(), "infrastructure_mode=iot must promote a default agent to report as iot_agent")
}

// TestInfrastructureModeIotDoesNotPromoteNonAgentBinaries verifies that
// infrastructure_mode=iot in shared config does not misclassify non-agent
// binaries (process-agent, trace-agent, etc.) as IoT.
func TestInfrastructureModeIotDoesNotPromoteNonAgentBinaries(t *testing.T) {
	mockConfig := configmock.New(t)

	originalFlavor := agentFlavor
	t.Cleanup(func() { agentFlavor = originalFlavor })

	mockConfig.SetInTest("infrastructure_mode", "iot")

	for _, nonAgentFlavor := range []string{ProcessAgent, TraceAgent, ClusterAgent, Dogstatsd} {
		agentFlavor = nonAgentFlavor
		assert.Equal(t, nonAgentFlavor, GetFlavor(), "infrastructure_mode=iot must not promote %q to iot_agent", nonAgentFlavor)
	}
}

func TestGetHumanReadableFlavor(t *testing.T) {
	// NOTE: This constructor is required to setup the global config as
	// a "mock" config that is using the "dynamic schema". Otherwise the function
	// SetFlavor in flavor.go will fail to modify the config due to its static schema.
	// TODO: Improve this by making flavor into a component that doesn't use
	// global state and doesn't call SetDefault.
	_ = configmock.New(t)
	for k, v := range agentFlavors {
		t.Run(fmt.Sprintf("%s: %s", k, v), func(t *testing.T) {
			SetFlavor(k)

			assert.Equal(t, v, GetHumanReadableFlavor())
		})
	}

	t.Run("Unknown Agent", func(t *testing.T) {
		SetFlavor("foo")

		assert.Equal(t, "Unknown Agent", GetHumanReadableFlavor())
	})
}
