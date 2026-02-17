// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package hostname

import (
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	osVM "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/e2e/client/agentclient"
)

type baseHostnameDriftSuite struct {
	e2e.BaseSuite[environments.Host]
}

func (v *baseHostnameDriftSuite) getSuiteOptions(osInstance osVM.Descriptor) []e2e.SuiteOption {
	agentConfig := "hostname_drift_initial_delay: 10s\nhostname_drift_recurring_interval: 15s"
	if osInstance.Family() == osVM.WindowsFamily {
		// Use EC2 hostname on Windows as well
		agentConfig += "\nec2_use_windows_prefix_detection: true"
	}
	var suiteOptions []e2e.SuiteOption
	suiteOptions = append(suiteOptions, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				ec2.WithAgentOptions(
					agentparams.WithAgentConfig(agentConfig),
				),
				ec2.WithEC2InstanceOptions(ec2.WithOS(osInstance)),
			),
		),
	))

	return suiteOptions
}

func (v *baseHostnameDriftSuite) TestHostnameDriftMetricsEmission() {
	// Wait for the agent to start and perform initial hostname detection
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Check that agent is ready
		assert.True(c, v.Env().Agent.Client.IsReady(), "Agent should be ready")
	}, 2*time.Minute, 10*time.Second, "Agent should be ready")

	// Verify that hostname drift metrics are emitted using agent telemetry
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Get telemetry metrics from agent
		output := v.Env().Agent.Client.Diagnose(agentclient.WithArgs([]string{"show-metadata", "agent-full-telemetry"}))

		// Check that we have drift resolution time metrics in the output
		assert.Contains(c, output, "drift_resolution_time_ms", "Should have drift_resolution_time_ms metrics in telemetry")

		// Check for specific metric components and their expected values
		assert.Contains(c, output, "provider=", "Should have provider tag in metrics")
		assert.Contains(c, output, "state=", "Should have state tag in metrics")

		// Assert specific provider values that should be present
		assert.Contains(c, output, "provider=\"aws\"", "Should have aws provider in metrics")

		// Assert specific state values that should be present
		assert.Contains(c, output, "state=\"no_drift\"", "Should have no_drift state in metrics")

	}, 2*time.Minute, 10*time.Second, "Hostname drift metrics should be emitted in agent telemetry")
}
