// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netflow contains e2e tests for netflow
package ha_agent

import (
	_ "embed"
	"path"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
	"github.com/stretchr/testify/assert"

	fakeintakeclient "github.com/DataDog/datadog-agent/test/fakeintake/client"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed config/netflowConfig.yaml
var datadogYaml string

// netflowDockerProvisioner defines a stack with a docker agent on an AmazonLinuxECS VM
// with the netflow-generator running and sending netflow payloads to the agent
func netflowDockerProvisioner() e2e.Provisioner {
	return e2e.NewTypedPulumiProvisioner[environments.DockerHost]("", func(ctx *pulumi.Context, env *environments.DockerHost) error {
		name := "netflowvm"
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, name, ec2.WithOS(os.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		host.Export(ctx, &env.RemoteHost.HostOutput)

		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, name)
		if err != nil {
			return err
		}
		fakeIntake.Export(ctx, &env.FakeIntake.FakeintakeOutput)

		filemanager := host.OS.FileManager()

		createConfigDirCommand, configPath, err := filemanager.TempDirectory("config")
		if err != nil {
			return err
		}

		// update agent datadog.yaml config content at fakeintake address resolution
		datadogYamlContent := fakeIntake.URL.ApplyT(func(url string) string {
			return strings.ReplaceAll(datadogYaml, "FAKEINTAKE_URL", url)
		}).(pulumi.StringOutput)

		configCommand, err := filemanager.CopyInlineFile(datadogYamlContent, path.Join(configPath, "datadog.yaml"),
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
		if err != nil {
			return err
		}

		dockerManager, err := docker.NewManager(&awsEnv, host, utils.PulumiDependsOn(installEcrCredsHelperCmd))
		if err != nil {
			return err
		}
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		envVars := pulumi.StringMap{"CONFIG_DIR": pulumi.String(configPath)}
		composeDependencies := []pulumi.Resource{configCommand}
		dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return err
	}, nil)
}

type netflowDockerSuite16 struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestHaAgentSuite runs the netflow e2e suite
func TestHaAgentSuite(t *testing.T) {
	e2e.Run(t, &netflowDockerSuite16{}, e2e.WithProvisioner(netflowDockerProvisioner()))
}

func (s *netflowDockerSuite16) TestHaAgentGroupTag_PresentOnDatadogAgentRunningMetric() {
	fakeClient := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		s.T().Logf("asserting datadog.agent.running metric has agent_group tag ...")
		metrics, err := fakeClient.FilterMetrics("datadog.agent.running")
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
		for _, metric := range metrics {
			s.T().Logf("datadog.agent.running metric: %v", metric)
		}

		tags := []string{"agent_group:test-group01"}
		metrics, err = fakeClient.FilterMetrics("datadog.agent.running", fakeintakeclient.WithTags[*aggregator.MetricSeries](tags))
		assert.NoError(c, err)
		assert.NotEmpty(c, metrics)
	}, 5*time.Minute, 10*time.Second)
}
