// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package hostname

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awshost "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
)

type hostnameDriftLinuxSuite struct {
	e2e.BaseSuite[environments.Host]
}

func TestHostnameDriftLinuxSuite(t *testing.T) {
	suiteParams := []e2e.SuiteOption{e2e.WithProvisioner(awshost.Provisioner())}

	e2e.Run(t, &hostnameDriftLinuxSuite{}, suiteParams...)
}

func (v *hostnameDriftLinuxSuite) TestHostnameDriftMetricsEmission() {
	// Configure agent with shorter drift check intervals for testing
	agentConfig := `hostname_drift_initial_delay: 30s
hostname_drift_recurring_interval: 60s`

	// Update environment with fake intake and agent config
	v.UpdateEnv(awshost.Provisioner(
		awshost.WithAgentOptions(agentparams.WithAgentConfig(agentConfig)),
	))

	// Wait for the agent to start and perform initial hostname detection
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Check that agent is running
		status := v.Env().Agent.Client.Status(agentclient.WithArgs([]string{"--json"}))
		assert.NotEmpty(c, status.Content, "Agent status should not be empty")
	}, 2*time.Minute, 10*time.Second, "Agent should be running")

	// Wait for the initial drift check to complete (initial delay + some buffer)
	time.Sleep(45 * time.Second)

	// Verify that hostname drift metrics are emitted using agent telemetry
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Get telemetry metrics from agent
		output, err := v.Env().RemoteHost.Execute("sudo datadog-agent diagnose show-metadata agent-full-telemetry | grep drift_resolution_time_ms")
		if !assert.NoError(c, err, "Should be able to execute diagnose command") {
			return
		}

		// Check that we have drift resolution time metrics in the output
		assert.Contains(c, output, "drift_resolution_time_ms", "Should have drift_resolution_time_ms metrics in telemetry")

		// Check for specific metric components
		assert.Contains(c, output, "provider=", "Should have provider tag in metrics")
		assert.Contains(c, output, "state=", "Should have state tag in metrics")

	}, 2*time.Minute, 10*time.Second, "Hostname drift metrics should be emitted in agent telemetry")
}
