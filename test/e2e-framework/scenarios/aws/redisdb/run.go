// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package redisdb contains the AWS Docker-on-EC2 scenario for the redisdb
// integration E2E lab.
//
// The scenario key is "aws/redisdb".  It provisions:
//   - one EC2 instance running Docker (via [ec2.NewVM] + [docker.NewAWSManager]);
//   - an optional ECS-Fargate FakeIntake (controlled by fakeintake options);
//   - a containerised Datadog Agent that carries the redisdb Compose manifest
//     as an extra sidecar stack, so the three Redis nodes and the load
//     generator start alongside the Agent on the same host.
//
// Usage – run directly via Pulumi by registering [Run] in the scenario
// registry, or call [DockerRun] as the pulumi.RunFunc entry-point.
package redisdb

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	redisdbapp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/redisdb"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	defaultVMName = "redisdb"
	scenarioTag   = "scenario:redisdb"
)

// Params holds all parameters needed to provision the redisdb scenario.
type Params struct {
	// Name is the logical name used to namespace Pulumi resources.
	Name string

	vmOptions         []ec2.VMOption
	agentOptions      []dockeragentparams.Option
	fakeintakeOptions []fakeintakescenario.Option
}

func newParams() *Params {
	// Non-nil slices mean "create this component"; nil means "skip".
	return &Params{
		Name:              defaultVMName,
		vmOptions:         []ec2.VMOption{},
		agentOptions:      []dockeragentparams.Option{},
		fakeintakeOptions: []fakeintakescenario.Option{},
	}
}

// Option is a function that modifies Params.
type Option func(*Params) error

// GetParams builds a Params by applying the supplied functional options.
func GetParams(opts ...Option) *Params {
	params := newParams()
	err := optional.ApplyOptions(params, opts)
	if err != nil {
		panic(fmt.Errorf("unable to apply Option, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment constructs Params by reading configuration values from
// the AWS environment's ConfigMap.  It mirrors the logic in the ec2docker
// scenario so that the same CI flags drive this scenario.
func ParamsFromEnvironment(e aws.Environment) *Params {
	p := newParams()

	// VM OS / AMI selection.
	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.UbuntuDefault)
	if img := e.InfraOSImageID(); img != "" {
		p.vmOptions = append(p.vmOptions, ec2.WithAMI(img, osDesc, osDesc.Architecture))
	} else {
		if e.InfraOSDescriptor() != "" {
			p.vmOptions = append(p.vmOptions, ec2.WithOS(osDesc))
		}
		if e.InfraOSImageIDUseLatest() {
			p.vmOptions = append(p.vmOptions, ec2.WithLatestAMI())
		}
	}

	// Agent image / version selection.
	if !e.AgentDeploy() {
		p.agentOptions = nil
	} else {
		if full := e.AgentFullImagePath(); full != "" {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithFullImagePath(full))
		} else if v := e.AgentVersion(); v != "" {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithImageTag(v))
		}
		if e.AgentJMX() {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithJMX())
		}
		if e.AgentFIPS() {
			p.agentOptions = append(p.agentOptions, dockeragentparams.WithFIPS())
		}
	}

	// FakeIntake options.
	if e.AgentUseFakeintake() {
		fiOpts := []fakeintakescenario.Option{}
		if e.InfraShouldDeployFakeintakeWithLB() {
			fiOpts = append(fiOpts, fakeintakescenario.WithLoadBalancer())
		}
		if retention := e.AgentFakeintakeRetentionPeriod(); retention != "" {
			fiOpts = append(fiOpts, fakeintakescenario.WithRetentionPeriod(retention))
		}
		p.fakeintakeOptions = fiOpts
	} else {
		p.fakeintakeOptions = nil
	}

	return p
}

// ---------------------------------------------------------------------------
// Functional options
// ---------------------------------------------------------------------------

// WithName overrides the logical name used to namespace Pulumi resources.
func WithName(name string) Option {
	return func(p *Params) error {
		p.Name = name
		return nil
	}
}

// WithEC2VMOptions appends options for the underlying EC2 VM.
func WithEC2VMOptions(opts ...ec2.VMOption) Option {
	return func(p *Params) error {
		p.vmOptions = append(p.vmOptions, opts...)
		return nil
	}
}

// WithAgentOptions appends options for the Datadog Docker Agent.
func WithAgentOptions(opts ...dockeragentparams.Option) Option {
	return func(p *Params) error {
		p.agentOptions = append(p.agentOptions, opts...)
		return nil
	}
}

// WithFakeIntakeOptions appends options for the ECS-Fargate FakeIntake.
func WithFakeIntakeOptions(opts ...fakeintakescenario.Option) Option {
	return func(p *Params) error {
		p.fakeintakeOptions = append(p.fakeintakeOptions, opts...)
		return nil
	}
}

// WithoutFakeIntake disables FakeIntake provisioning.
func WithoutFakeIntake() Option {
	return func(p *Params) error {
		p.fakeintakeOptions = nil
		return nil
	}
}

// WithoutAgent disables Datadog Agent provisioning.
func WithoutAgent() Option {
	return func(p *Params) error {
		p.agentOptions = nil
		return nil
	}
}

// ---------------------------------------------------------------------------
// Core provisioning logic
// ---------------------------------------------------------------------------

// Run provisions the complete redisdb E2E environment into the given Pulumi
// context.  env must implement [outputs.DockerHostOutputs]; it is satisfied by
// both [outputs.DockerHost] (lightweight, no test-client deps) and the
// test-provisioner's environments.DockerHost.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.DockerHostOutputs, params *Params) error {
	// 1. EC2 Docker host.
	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}
	if err = host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// 2. Docker manager (installs Docker on the host and exposes helpers).
	manager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}
	if err = manager.Export(ctx, env.DockerOutput()); err != nil {
		return err
	}

	// 3. Optional FakeIntake on ECS Fargate.
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintakescenario.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, env.FakeIntakeOutput()); err != nil {
			return err
		}

		// Wire FakeIntake into the Agent when both are enabled.
		if params.agentOptions != nil {
			newOpts := []dockeragentparams.Option{dockeragentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
	} else {
		env.DisableFakeIntake()
	}

	// 4. Datadog Agent with the redisdb Compose manifest attached.
	if params.agentOptions != nil {
		// Scenario-specific tags: stack identity + integration name.
		params.agentOptions = append(params.agentOptions,
			dockeragentparams.WithTags([]string{
				"stackid:" + ctx.Stack(),
				scenarioTag,
			}),
		)

		loadAssetResources, err := manager.Host.OS.FileManager().CopyFSFolder(
			redisdbapp.LoadAssets,
			redisdbapp.LoadAssetsPath,
			redisdbapp.RemoteAssetsPath,
		)
		if err != nil {
			return err
		}

		// Always attach the redisdb topology so the Redis nodes and the load
		// generator start alongside the Agent on every deployment.
		params.agentOptions = append(params.agentOptions,
			dockeragentparams.WithPulumiDependsOn(utils.PulumiDependsOn(loadAssetResources...)),
			dockeragentparams.WithExtraComposeManifest(
				redisdbapp.DockerComposeManifest.Name,
				redisdbapp.DockerComposeManifest.Content,
			),
		)

		ddAgent, err := agent.NewDockerAgent(&awsEnv, host, manager, params.agentOptions...)
		if err != nil {
			return err
		}
		if err = ddAgent.Export(ctx, env.DockerAgentOutput()); err != nil {
			return err
		}
	} else {
		env.DisableAgent()
	}

	return nil
}

// DockerRun is the pulumi.RunFunc entry-point for the "aws/redisdb" scenario.
// It creates a lightweight [outputs.DockerHost] (no test-client imports) and
// delegates to [Run].
func DockerRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewDockerHost()

	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
