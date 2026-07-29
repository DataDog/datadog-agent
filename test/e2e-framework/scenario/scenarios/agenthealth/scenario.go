// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agenthealth

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	e2eos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenario"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/docker_permission_agent_config.yaml
var baseAgentConfig string

//go:embed fixtures/docker-compose.busybox.yaml
var busyboxCompose string

// osDescriptor maps the OS enum string to the framework descriptor.
func osDescriptor(name string) (e2eos.Descriptor, error) {
	switch name {
	case "ubuntu-22.04":
		return e2eos.Ubuntu2204E2E, nil
	case "debian-12":
		return e2eos.Debian12, nil
	case "amazon-linux-2023":
		return e2eos.AmazonLinux2023, nil
	default:
		return e2eos.Descriptor{}, fmt.Errorf("unknown os %q", name)
	}
}

// archFromString maps the arch enum string to the framework Architecture type.
func archFromString(arch string) (e2eos.Architecture, error) {
	switch arch {
	case "x86_64", "":
		return e2eos.AMD64Arch, nil
	case "arm64":
		return e2eos.ARM64Arch, nil
	default:
		return "", fmt.Errorf("unknown arch %q (valid: x86_64, arm64)", arch)
	}
}

func provision(p *Params) provisioners.PulumiEnvRunFunc[Env] {
	return func(ctx *pulumi.Context, env *Env) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		desc, err := osDescriptor(p.OS)
		if err != nil {
			return err
		}
		arch, err := archFromString(p.Arch)
		if err != nil {
			return err
		}

		remoteHost, err := ec2.NewVM(awsEnv, "agent-health", ec2.WithOSArch(desc, arch))
		if err != nil {
			return err
		}
		if err = remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		fi, err := fakeintake.NewECSFargateInstance(awsEnv, "agent-health", fakeintake.WithoutDDDevForwarding())
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

		composeBusyboxCmd, err := dockerManager.ComposeStrUp("busybox", []docker.ComposeInlineManifest{
			{Name: "busybox", Content: pulumi.String(busyboxCompose)},
		}, pulumi.StringMap{})
		if err != nil {
			return err
		}

		// Layer scenario-intrinsic base options first, then user-supplied options so
		// the caller can override agent-version, flavor, pipeline-id, etc.
		agentOpts := []agentparams.Option{
			agentparams.WithAgentConfig(baseAgentConfig),
			agentparams.WithFakeintake(fi),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeBusyboxCmd})),
		}
		userOpts, err := p.Agent.ToOptions()
		if err != nil {
			return err
		}
		agentOpts = append(agentOpts, userOpts...)

		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost, agentOpts...)
		if err != nil {
			return err
		}
		return hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
	}
}

// Provisioner adapts Params to a typed provisioner for the custom Env.
func Provisioner(p *Params) (provisioners.TypedProvisioner[Env], error) {
	return provisioners.NewTypedPulumiProvisioner("agent-health", provision(p), nil), nil
}

// Scenario returns the unified agent-health scenario definition.
func Scenario() scenario.Scenario[Env] {
	return scenario.Scenario[Env]{
		Name:        "agent-health",
		Description: "VM with host Agent + dockerized workload reporting to fakeintake",
		NewParams:   func() any { return NewParams() },
		Provisioner: func(a any) (provisioners.TypedProvisioner[Env], error) {
			return Provisioner(a.(*Params))
		},
		Actions: map[string]scenario.Action[Env]{
			"connection-info": {
				Description: "Print SSH connection details for the VM",
				Run: func(_ context.Context, e *Env, _ any) error {
					h := e.RemoteHost.HostOutput
					fmt.Printf("ssh %s@%s -p %d\n", h.Username, h.Address, h.Port)
					return nil
				},
			},
			"restart-agent": {
				Description: "Restart the Datadog Agent",
				Run: func(_ context.Context, e *Env, _ any) error {
					return e.Agent.Client.Restart()
				},
			},
		},
	}
}

// Register registers the agent-health scenario in the package registry.
func Register() { scenario.Register(Scenario()) }
