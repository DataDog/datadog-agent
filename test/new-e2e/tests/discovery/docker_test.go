// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"

	scendocker "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/docker"
)

type dockerTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDockerTestSuite(t *testing.T) {
	t.Parallel()

	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_DISCOVERY_ENABLED", pulumi.StringPtr("true")),
		dockeragentparams.WithAgentServiceEnvVariable("DD_DISCOVERY_USE_SYSTEM_PROBE_LITE", pulumi.StringPtr("true")),
		// Setting any DD_SYSTEM_PROBE_* env var triggers privileged mode in the
		// Docker compose. This var sets the socket path to its default (no-op)
		// and is not in system-probe-lite's NON_DISCOVERY_ENV_VARS list, so it
		// won't cause fallback to full system-probe.
		dockeragentparams.WithAgentServiceEnvVariable("DD_SYSTEM_PROBE_CONFIG_SYSPROBE_SOCKET", pulumi.StringPtr("/opt/datadog-agent/run/sysprobe.sock")),
	}

	options := []e2e.SuiteOption{
		e2e.WithProvisioner(awsdocker.Provisioner(
			awsdocker.WithRunOptions(
				scendocker.WithAgentOptions(agentOpts...),
			),
		)),
	}

	e2e.Run(t, &dockerTestSuite{}, options...)
}

func (s *dockerTestSuite) TestSystemProbeLiteRunning() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		ps := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "ps", "aux")
		assert.True(c, strings.Contains(ps, "system-probe-lite"),
			"system-probe-lite should be running in the container, got:\n%s", ps)
	}, 2*time.Minute, 10*time.Second)
}
