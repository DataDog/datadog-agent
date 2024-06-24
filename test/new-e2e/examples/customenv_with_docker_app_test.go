// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package examples

import (
	_ "embed"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/test-infra-definitions/components/docker"
	"github.com/DataDog/test-infra-definitions/components/os"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/agentparams"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type vmPlusDockerEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker

	// additional resources
	remoteHostLogsDir string
}

//go:embed testfixtures/docker-compose.lighttpd.yaml
var lighttpdComposeContent string

//go:embed testfixtures/lighttpd.conf
var lighttpdConfigContent string

//go:embed testfixtures/lighttpd_integration.conf.yaml
var lighttpdIntegrationConfigContent string

func vmPlusDockerEnvProvisioner() e2e.PulumiEnvRunFunc[vmPlusDockerEnv] {
	return func(ctx *pulumi.Context, env *vmPlusDockerEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// First we create a remote host with Amazon Linux ECS, that comes with Docker pre-installed
		remoteHost, err := ec2.NewVM(awsEnv, "main", ec2.WithOS(os.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		// we export it to env.RemoteHost, this will automatically initialize the ssh client on env.RemoteHost
		remoteHost.Export(ctx, &env.RemoteHost.HostOutput)

		// create a fakeintake instance on ECS Fargate
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "")
		if err != nil {
			return err
		}
		// export its configuration to the environment, this will automatically initialize the fakeintake client
		err = fakeIntake.Export(ctx, &env.Fakeintake.FakeintakeOutput)
		if err != nil {
			return err
		}

		// Create a docker manager
		dockerManager, err := docker.NewManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		// export the docker manager configurartion to the environment, this will automatically initialize the docker client
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		// let's create a lighttpd container
		// first we create a config file and a directory for log files
		// that we will mount in the container
		// files will be created eventually, our docker compose needs to wait for them
		// we track the creation of files in an array of pulumi resources
		mountedFilesCommands := make([]pulumi.Resource, 0, 2)
		// create a tmp directory on the remote host, we will use it to store the configuration file
		// and to read log files
		// lighttpdDirCmd is the command to create the directory
		// it will be executed on the remote host, we can run docker compose
		// after it is done
		remoteTmpDirCmd, lighttpdDir, err := remoteHost.OS.FileManager().TempDirectory("lighttpd")
		if err != nil {
			return err
		}
		// export the remote directory path to the environment
		env.remoteHostLogsDir = lighttpdDir
		mountedFilesCommands = append(mountedFilesCommands, remoteTmpDirCmd)
		// write the lighttpd configuration file
		lighttpdConfigFile := lighttpdDir + "/lighttpd.conf"
		lighttpdConfigFileCmd, err := remoteHost.OS.FileManager().CopyInlineFile(pulumi.String(lighttpdConfigContent), lighttpdConfigFile)
		if err != nil {
			return err
		}
		mountedFilesCommands = append(mountedFilesCommands, lighttpdConfigFileCmd)

		// the name is internal only
		envVars := pulumi.StringMap{
			"DD_LIGHTTPD_CONFIG":    pulumi.String(lighttpdConfigFile),
			"DD_LIGHTTPD_LOGS_PATH": pulumi.String(lighttpdDir),
		}
		// compose lighttpd
		composeLighttpdCmd, err := dockerManager.ComposeStrUp("lighttpd", []docker.ComposeInlineManifest{
			{
				Name:    "lighttpd",
				Content: pulumi.String(lighttpdComposeContent),
			},
		}, envVars, pulumi.DependsOn(mountedFilesCommands))
		if err != nil {
			return err
		}
		// replace `ACCESS_LOG_PATH` and `ERROR_LOG_PATH` in the integration config
		lighttpdIntegrationConfigContent = strings.ReplaceAll(lighttpdIntegrationConfigContent, "LIGHTTPD_LOG_PATH", lighttpdDir)
		// install the agent on the remote host
		agent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithIntegration("lighttpd.d", lighttpdIntegrationConfigContent),
			agentparams.WithLogs(),
			// agent depends on the docker compose command
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeLighttpdCmd})),
		)
		if err != nil {
			return err
		}
		err = agent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}
		return nil
	}
}

type vmPlusDockerEnvSuite struct {
	e2e.BaseSuite[vmPlusDockerEnv]
}

func TestLighttpdOnDockerFromHost(t *testing.T) {
	e2e.Run(t, &vmPlusDockerEnvSuite{}, e2e.WithPulumiProvisioner(vmPlusDockerEnvProvisioner(), nil))
}

func (v *vmPlusDockerEnvSuite) TestAgentMonitorsLighttpd() {
	assert.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		logs, err := v.Env().Fakeintake.Client().FilterLogs("lighttpd")
		assert.NoError(c, err)
		assert.NotEmpty(c, logs)
	}, 5*time.Minute, 10*time.Second)
	assert.True(v.T(), v.Env().Agent.Client.IsReady())
	// ExecuteCommand executes a command on containerName and returns the output
	assert.NotEmpty(v.T(), v.Env().Docker.Client.ExecuteCommand("lighttpd", "ls"))
}
