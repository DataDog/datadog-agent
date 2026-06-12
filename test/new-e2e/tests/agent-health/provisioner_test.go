// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	_ "embed"
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/installers/hostagent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/docker_permission_agent_config.yaml
var dockerPermissionAgentConfig string

//go:embed fixtures/docker-compose.busybox.yaml
var busyboxComposeContent string

// dockerPermissionEnvProvisioner returns a TypedProvisioner for the docker
// permission test environment. Agent installation is performed via SSH in
// PostProvision (after Pulumi deploys the VM, FakeIntake, and Docker compose),
// preserving the dependency ordering without Pulumi DependsOn.
func dockerPermissionEnvProvisioner() provisioners.TypedProvisioner[dockerPermissionEnv] {
	pulumiProv := provisioners.NewTypedPulumiProvisioner("docker-permission", func(ctx *pulumi.Context, env *dockerPermissionEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "dockervm")
		if err != nil {
			return err
		}
		if err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		fi, err := fakeintake.NewECSFargateInstance(awsEnv, "", fakeintake.WithoutDDDevForwarding())
		if err != nil {
			return err
		}
		if err = fi.Export(ctx, &env.Fakeintake.FakeintakeOutput); err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		if err = dockerManager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		// Deploy the busybox compose — the agent must not start until this is done.
		// PostProvision runs after all Pulumi resources are ready, so the ordering
		// is preserved without needing an explicit DependsOn on the agent install.
		if _, err = dockerManager.ComposeStrUp("busybox", []docker.ComposeInlineManifest{
			{Name: "busybox", Content: pulumi.String(busyboxComposeContent)},
		}, pulumi.StringMap{}); err != nil {
			return err
		}

		// Agent installation moved to PostProvision.
		env.Agent = nil
		return nil
	}, nil)

	return provisioners.WithPostProvision(pulumiProv, func(t *testing.T, env *dockerPermissionEnv) {
		env.Agent = hostagent.InstallOnHost(
			installers.FromT(t),
			env.RemoteHost,
			env.Fakeintake,
			agentparams.WithAgentConfig(dockerPermissionAgentConfig),
		)
	})
}
