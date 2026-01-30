// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	_ "embed"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/docker_permission_agent_config.yaml
var dockerPermissionAgentConfig string

//go:embed fixtures/docker-compose.busybox.yaml
var busyboxComposeContent string

type dockerPermissionEnv struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
	Docker     *components.RemoteHostDocker
}

func dockerPermissionEnvProvisioner() provisioners.PulumiEnvRunFunc[dockerPermissionEnv] {
	return func(ctx *pulumi.Context, env *dockerPermissionEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Create a remote host
		remoteHost, err := ec2.NewVM(awsEnv, "dockervm")
		if err != nil {
			return err
		}
		err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput)
		if err != nil {
			return err
		}

		// Create a fakeintake instance on ECS Fargate
		// Skip forwarding to dddev, agenthealth is only on staging
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
		if err != nil {
			return err
		}
		err = fakeIntake.Export(ctx, &env.Fakeintake.FakeintakeOutput)
		if err != nil {
			return err
		}

		// Create a docker manager
		dockerManager, err := docker.NewManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		err = dockerManager.Export(ctx, &env.Docker.ManagerOutput)
		if err != nil {
			return err
		}

		// Deploy busybox containers using Docker Compose
		// These will run without proper permissions to trigger the docker permission issue
		composeBusyboxCmd, err := dockerManager.ComposeStrUp("busybox", []docker.ComposeInlineManifest{
			{
				Name:    "busybox",
				Content: pulumi.String(busyboxComposeContent),
			},
		}, nil)
		if err != nil {
			return err
		}

		// Install the agent on the remote host
		// Agent depends on containers being deployed first
		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithFakeintake(fakeIntake),
			agentparams.WithAgentConfig(dockerPermissionAgentConfig),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeBusyboxCmd})),
		)
		if err != nil {
			return err
		}
		err = hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
		if err != nil {
			return err
		}

		return nil
	}
}
