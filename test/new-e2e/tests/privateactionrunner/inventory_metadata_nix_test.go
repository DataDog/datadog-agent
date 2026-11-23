// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
)

type linuxPARInventoryMetadataSuite struct {
	e2e.BaseSuite[environments.Host]
}

// TestLinuxPARInventoryMetadataSuite verifies that the inventory-agent metadata
// payload submitted to the backend reports the Private Action Runner as enabled
// when both private_action_runner.enabled and private_action_runner.split_enabled
// are set. Unlike the in-memory show-metadata command, this asserts the payload
// actually captured by fakeintake at /api/v1/metadata.
func TestLinuxPARInventoryMetadataSuite(t *testing.T) {
	t.Parallel()
	urn, privateKey := GenerateTestRunnerIdentity(t)
	config := splitConfig(urn, privateKey)
	e2e.Run(t, &linuxPARInventoryMetadataSuite{}, e2e.WithProvisioner(
		awshost.Provisioner(
			awshost.WithRunOptions(
				scenec2.WithAgentOptions(
					agentparams.WithAgentConfig(config),
					agentparams.WithFile("/etc/datadog-agent/environment", "DD_INTERNAL_PAR_USE_DD_URL_FOR_OPMS=true\n", true),
				),
			),
		),
	))
}

// TestFeaturePrivateActionRunnerEnabledReported asserts the core agent submits
// feature_private_action_runner_enabled=true in its inventory-agent metadata.
func (s *linuxPARInventoryMetadataSuite) TestFeaturePrivateActionRunnerEnabledReported() {
	client := s.Env().FakeIntake.Client()

	s.Require().EventuallyWithT(func(c *assert.CollectT) {
		payloads, err := client.GetMetadata()
		require.NoError(c, err)

		// Fakeintake retains earlier payloads. Assert the current inventory state
		// so a payload from before the configured Agent restart cannot fail every poll.
		latest := -1
		for i, payload := range payloads {
			if payload.Metadata == nil {
				continue
			}
			if latest == -1 || payload.Timestamp > payloads[latest].Timestamp {
				latest = i
			}
		}
		require.NotEqual(c, -1, latest, "no inventory-agent metadata payload received yet")

		value, ok := payloads[latest].Metadata["feature_private_action_runner_enabled"]
		require.True(c, ok, "feature_private_action_runner_enabled missing from the latest inventory-agent payload")
		assert.Equal(c, true, value, "feature_private_action_runner_enabled should be true when private_action_runner.enabled and split_enabled are set")
	}, 5*time.Minute, 10*time.Second)
}
