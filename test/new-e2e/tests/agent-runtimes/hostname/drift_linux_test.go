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
	e2e.Run(t, &hostnameDriftLinuxSuite{}, e2e.WithProvisioner(awshost.Provisioner()))
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

	// Verify that hostname drift metrics are emitted
	v.EventuallyWithT(func(c *assert.CollectT) {
		// Get metrics from fake intake
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("datadog.agent.hostname.drift_detected")
		if !assert.NoError(c, err, "Should be able to query drift_detected metrics") {
			return
		}

		// Check that we have at least one drift detection metric
		// Note: The metric might be 0 if no drift was detected, but the metric should exist
		assert.NotEmpty(c, metrics, "Should have drift_detected metrics")

		// Also check for resolution time metrics
		resolutionMetrics, err := v.Env().FakeIntake.Client().FilterMetrics("datadog.agent.hostname.drift_resolution_time_ms")
		if !assert.NoError(c, err, "Should be able to query drift_resolution_time_ms metrics") {
			return
		}

		assert.NotEmpty(c, resolutionMetrics, "Should have drift_resolution_time_ms metrics")

	}, 2*time.Minute, 10*time.Second, "Hostname drift metrics should be emitted")

	// Verify that the metrics have the expected tags
	v.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := v.Env().FakeIntake.Client().FilterMetrics("datadog.agent.hostname.drift_detected")
		if !assert.NoError(c, err, "Should be able to query drift_detected metrics") {
			return
		}

		if len(metrics) > 0 {
			// Check that the metric has the expected tags
			metric := metrics[len(metrics)-1] // Get the latest metric
			tags := metric.GetTags()

			// Should have state and provider tags
			hasState := false
			hasProvider := false
			for _, tag := range tags {
				if len(tag) > 6 && tag[:6] == "state:" {
					hasState = true
				}
				if len(tag) > 9 && tag[:9] == "provider:" {
					hasProvider = true
				}
			}

			assert.True(c, hasState, "Drift metric should have state tag")
			assert.True(c, hasProvider, "Drift metric should have provider tag")
		}

	}, 1*time.Minute, 5*time.Second, "Drift metrics should have expected tags")
}
