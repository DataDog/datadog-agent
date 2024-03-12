// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package netflow contains e2e tests for netflow
package netflow

import (
	_ "embed"
	"path"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/dockeragentparams"
	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed compose/netflowCompose.yaml
var netflowCompose string

//go:embed config/netflowConfig.yaml
var netflowConfig string

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
		// edit config file
		dontUseSudo := false
		configCommand, err := filemanager.CopyInlineFile(pulumi.String(netflowConfig), path.Join(configPath, "snmp.yaml"), dontUseSudo, // JMWNAME
			pulumi.DependsOn([]pulumi.Resource{createConfigDirCommand}))
		if err != nil {
			return err
		}

		dockerManager, _, err := docker.NewManager(*awsEnv.CommonEnvironment, host, false)
		if err != nil {
			return err
		}

		envVars := pulumi.StringMap{"CONFIG_DIR": pulumi.String(configPath)}
		composeDependencies := []pulumi.Resource{configCommand}
		dockerAgent, err := agent.NewDockerAgent(*awsEnv.CommonEnvironment, host, dockerManager,
			dockeragentparams.WithFakeintake(fakeIntake),
			dockeragentparams.WithExtraComposeManifest("netflow-generator", pulumi.String(netflowCompose)),
			dockeragentparams.WithEnvironmentVariables(envVars),
			dockeragentparams.WithPulumiDependsOn(pulumi.DependsOn(composeDependencies)),
			// JMW add dependency on netflow-generator container starting?
		)
		if err != nil {
			return err
		}
		dockerAgent.Export(ctx, &env.Agent.DockerAgentOutput)

		return err
	}, nil)
}

type netflowDockerSuite struct {
	e2e.BaseSuite[environments.DockerHost]
}

// TestNetflowSuite runs the netflow e2e suite
func TestNetflowSuite(t *testing.T) {
	e2e.Run(t, &netflowDockerSuite{}, e2e.WithProvisioner(netflowDockerProvisioner()))
	//JMW e2e.Run(t, &netflowDockerSuite{}, e2e.WithProvisioner(netflowDockerProvisioner()), e2e.WithSkipDeleteOnFailure())
	//JMW e2e.Run(t, &netflowDockerSuite{}, e2e.WithProvisioner(netflowDockerProvisioner()), e2e.WithDevMode())
}

// TestNetflow tests that the netflow-generator container is running and that the agent container
// is sending netflow data to the fakeintake
func (s *netflowDockerSuite) TestNetflow() {
	fakeintake := s.Env().FakeIntake.Client()
	s.EventuallyWithT(func(c *assert.CollectT) {
		metrics, err := fakeintake.GetMetricNames()
		s.T().Logf("JMW fakeintake.GetMetricNames(): %v", metrics)
		assert.NoError(c, err)

		/* JMW test currently fails with this check for datadog.netflow.aggregator.flow_flushed

				ssh ec2-user@10.1.60.188
				[ec2-user@ip-10-1-60-188 ~]$ docker ps -a
				CONTAINER ID   IMAGE                            COMMAND                  CREATED          STATUS                      PORTS                                                                                  NAMES
				a5f2bd923984   networkstatic/nflow-generator    "/usr/local/bin/nflo…"   2 minutes ago    Exited (1) 2 minutes ago                                                                                           dd-netflow

				[ec2-user@ip-10-1-60-188 ~]$ docker logs dd-netflow
				time="2024-03-13T01:17:24Z" level=info msg="sending netflow data to a collector ip: datadog-agent and port: 2056. \nUse ctrl^c to terminate the app."
				time="2024-03-13T01:17:25Z" level=fatal msg="Error connecting to the target collector: write udp 172.18.0.3:52712->172.18.0.2:2056: write: connection refused"

				1) do we need to expose port 2056 from the agent???

				AND/OR

				2) why is netflow not enabled?

				[ec2-user@ip-10-1-60-188 ~]$ docker exec datadog-agent agent config | grep -A1 netflow
		        netflow:
		          enabled: "false"
		*/
		assert.Contains(c, metrics, "datadog.netflow.aggregator.flows_flushed", "metrics %v doesn't contain datadog.netflow.aggregator.flows_flushed", metrics)
		assert.Contains(c, metrics, "datadog.agent.running", "metrics %v doesn't contain datadog.agent.running", metrics)
	}, 5*time.Minute, 10*time.Second)
}
