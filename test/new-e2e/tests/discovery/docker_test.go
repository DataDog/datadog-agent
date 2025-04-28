// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package discovery

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/testutil/flake"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	awsdocker "github.com/DataDog/datadog-agent/test/new-e2e/pkg/provisioners/aws/docker"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/e2e/client/agentclient"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pythonImage = "public.ecr.aws/docker/library/python:3"
)

type dockerDiscoveryTestSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

func TestDiscoveryDocker(t *testing.T) {
	agentOpts := []dockeragentparams.Option{
		dockeragentparams.WithAgentServiceEnvVariable("DD_DISCOVERY_ENABLED", pulumi.StringPtr("true")),
	}

	e2e.Run(t,
		&dockerDiscoveryTestSuite{},
		e2e.WithProvisioner(
			awsdocker.Provisioner(
				awsdocker.WithAgentOptions(agentOpts...),
			)))
}

func (s *dockerDiscoveryTestSuite) TestServiceDiscoveryContainerID() {
	t := s.T()

	flake.Mark(t)

	client := s.Env().FakeIntake.Client()
	err := client.FlushServerAndResetAggregators()
	require.NoError(t, err)

	s.assertDockerAgentDiscoveryRunning()

	_, err = s.Env().RemoteHost.Execute("docker pull " + pythonImage)
	if err != nil {
		s.T().Skipf("could not pull docker image for service discovery E2E test: %s", err)
	}

	containerID := s.Env().RemoteHost.MustExecute("docker run -d --name e2e-test-python-server --publish 8090:8090 " + pythonImage + " python -m http.server 8090")
	t.Cleanup(func() {
		s.Env().RemoteHost.MustExecute("docker stop e2e-test-python-server && docker rm e2e-test-python-server")
	})
	containerID = strings.TrimSuffix(containerID, "\n")
	t.Logf("service container ID: %v", containerID)

	services := s.Env().Docker.Client.ExecuteCommand(s.Env().Agent.ContainerName, "curl", "-s", "--unix-socket", "/opt/datadog-agent/run/sysprobe.sock", "http://unix/discovery/check")
	t.Logf("system-probe services: %v", services)

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		payloads, err := client.GetServiceDiscoveries()
		require.NoError(t, err)

		foundMap := make(map[string]*aggregator.ServiceDiscoveryPayload)
		for _, p := range payloads {
			name := p.Payload.ServiceName
			t.Log("RequestType", p.RequestType, "ServiceName", name)

			if p.RequestType == "start-service" {
				foundMap[name] = p
			}
		}

		require.NotEmpty(c, foundMap)
		require.Contains(c, foundMap, "http.server")
		require.Equal(c, containerID, foundMap["http.server"].Payload.ContainerID)
	}, 3*time.Minute, 10*time.Second)
}

func (s *dockerDiscoveryTestSuite) assertDockerAgentDiscoveryRunning() {
	t := s.T()

	assert.EventuallyWithT(t, func(c *assert.CollectT) {
		statusOutput := s.Env().Agent.Client.Status(agentclient.WithArgs([]string{"collector", "--json"})).Content
		assertCollectorStatusFromJSON(c, statusOutput, "service_discovery")
	}, 2*time.Minute, 10*time.Second)
}
