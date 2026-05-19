// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package milvus

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	commonconfig "github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/runner"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const milvusCompose = `services:
  milvus:
    image: milvusdb/milvus:v2.5.4
    container_name: milvus
    command: ["milvus", "run", "standalone"]
    environment:
      ETCD_USE_EMBED: "true"
      ETCD_DATA_DIR: /var/lib/milvus/etcd
      COMMON_STORAGETYPE: local
    ports:
      - "19530:19530"
      - "9091:9091"
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9091/metrics"]
      interval: 10s
      timeout: 5s
      retries: 30
`

const milvusIntegrationConfig = `init_config:

instances:
  - openmetrics_endpoint: http://localhost:9091/metrics
    tags:
      - service:milvus
`

const agentConfig = `logs_enabled: false
`

type milvusEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Docker     *components.RemoteHostDocker
}

func milvusEnvProvisioner() provisioners.PulumiEnvRunFunc[milvusEnv] {
	return func(ctx *pulumi.Context, env *milvusEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "main", ec2.WithOS(e2eos.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		remoteHost.Export(ctx, &env.RemoteHost.HostOutput)

		dockerManager, err := docker.NewAWSManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		if err := dockerManager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		composeMilvusCmd, err := dockerManager.ComposeStrUp("milvus", []docker.ComposeInlineManifest{
			{
				Name:    "milvus",
				Content: pulumi.String(milvusCompose),
			},
		}, pulumi.StringMap{})
		if err != nil {
			return err
		}

		// No fakeintake is attached: the Agent uses the API key from the e2e runner
		// secret store (usually populated by dd-auth) and sends metrics to the Datadog API.
		agent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("milvus.d", milvusIntegrationConfig),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeMilvusCmd})),
		)
		if err != nil {
			return err
		}
		return agent.Export(ctx, &env.Agent.HostAgentOutput)
	}
}

type milvusSuite struct {
	e2e.BaseSuite[milvusEnv]
}

func TestMilvus(t *testing.T) {
	t.Parallel()

	configMap := runner.ConfigMap{}
	configMap.Set(commonconfig.DDAgentConfigNamespace+":"+commonconfig.DDAgentFakeintake, "false", false)

	e2e.Run(t, &milvusSuite{}, e2e.WithPulumiProvisioner(milvusEnvProvisioner(), configMap))
}

func (s *milvusSuite) TestLabAccessInfo() {
	s.T().Logf("Milvus lab host: ssh %s@%s -p %d", s.Env().RemoteHost.Username, s.Env().RemoteHost.Address, s.Env().RemoteHost.Port)
	s.T().Log("Milvus metrics endpoint on host: http://localhost:9091/metrics")
	s.T().Log("Useful commands: sudo datadog-agent status; sudo datadog-agent status collector --json; docker logs milvus")
}

func (s *milvusSuite) TestMilvusContainerIsRunning() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		containers, err := s.Env().Docker.Client.ListContainers()
		require.NoError(c, err)
		assert.Contains(c, containers, "milvus")

		metrics, err := s.Env().RemoteHost.Execute("curl -sf http://localhost:9091/metrics")
		require.NoError(c, err)
		assert.Contains(c, metrics, "milvus")
	}, 5*time.Minute, 10*time.Second)
}

func (s *milvusSuite) TestAgentRunsMilvusCheck() {
	assert.EventuallyWithT(s.T(), func(c *assert.CollectT) {
		require.True(c, s.Env().Agent.Client.IsReady())

		status, err := s.Env().RemoteHost.Execute("sudo datadog-agent status collector --json")
		require.NoError(c, err)
		assert.Contains(c, status, "milvus")
		assert.NotContains(c, strings.ToLower(status), "connection refused")
	}, 5*time.Minute, 10*time.Second)
}
