// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package process

import (
	"testing"
	"time"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

// infrastructureModeTag is the mark an Agent running in cloud_cost_only mode
// stamps on the containers it reports, so that each backend consumer can tell
// them from the containers of a fully monitored host.
const infrastructureModeTag = "infra_mode:cloud_cost_only"

// infraModeSuite covers the mark on the containers carried by the process
// payload. Those containers come from the same provider the container check
// reports, so asserting them here covers both payloads.
//
// It is a separate entry point rather than a method on dockerTestSuite because
// infrastructure_mode is Agent-wide: setting it there would run every existing
// assertion under a non-default configuration.
type infraModeSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestInfraModeSuite(t *testing.T) {
	t.Parallel()

	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_INFRASTRUCTURE_MODE", pulumi.StringPtr("cloud_cost_only")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_PROCESS_CONFIG_PROCESS_COLLECTION_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithExtraComposeManifest("fakeProcess", pulumi.String(fakeProcessCompose)),
	}

	e2e.Run(t, &infraModeSuite{}, e2e.WithProvisioner(awsdocker.Provisioner(
		awsdocker.WithRunOptions(scendocker.WithAgentOptions(agentOpts...)),
	)))
}

func (s *infraModeSuite) TestContainersCarryInfraMode() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		assertRunningChecks(c, s.Env().Agent.Client, []string{"process", "rtprocess", "service_discovery"}, false)
	}, 1*time.Minute, 5*time.Second)

	// Drop the payloads sent before the checks settled.
	require.NoError(t, s.Env().FakeIntake.Client().FlushServerAndResetAggregators())

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := s.Env().FakeIntake.Client().GetProcesses()
		assert.NoError(c, err, "failed to get process payloads from fakeintake")

		containers := collectContainersByName(payloads, "fake-process")
		if !assert.NotEmpty(c, containers, "fake-process container not found in payloads") {
			return
		}
		for _, container := range containers {
			assert.Contains(c, container.Tags, infrastructureModeTag)
		}
	}, 2*time.Minute, 10*time.Second)
}
