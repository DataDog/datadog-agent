// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package milvus contains the definition of an AWS Docker environment running Milvus.
package milvus

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	milvusapp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/milvus"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Run deploys a Docker host, a Milvus standalone stack with a load generator,
// and a containerized Datadog Agent configured through Autodiscovery labels to
// run the Milvus integration.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	env := outputs.NewDockerHost()

	host, err := ec2.NewVM(awsEnv, "milvus", vmOptionsFromEnvironment(awsEnv)...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	manager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}
	if err := manager.Export(ctx, env.DockerOutput()); err != nil {
		return err
	}

	agentOptions := agentOptionsFromEnvironment(awsEnv)
	if awsEnv.AgentUseFakeintake() {
		fiOpts := []fakeintake.Option{}
		if awsEnv.InfraShouldDeployFakeintakeWithLB() {
			fiOpts = append(fiOpts, fakeintake.WithLoadBalancer())
		}
		if retention := awsEnv.AgentFakeintakeRetentionPeriod(); retention != "" {
			fiOpts = append(fiOpts, fakeintake.WithRetentionPeriod(retention))
		}
		fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "milvus", fiOpts...)
		if err != nil {
			return err
		}
		if err := fakeIntake.Export(ctx, env.FakeIntakeOutput()); err != nil {
			return err
		}
		agentOptions = append([]dockeragentparams.Option{dockeragentparams.WithFakeintake(fakeIntake)}, agentOptions...)
	} else {
		env.DisableFakeIntake()
	}

	if !awsEnv.AgentDeploy() {
		env.DisableAgent()
		return nil
	}

	agentOptions = append(agentOptions,
		dockeragentparams.WithTags([]string{"stackid:" + ctx.Stack(), "scenario:milvus"}),
		dockeragentparams.WithExtraComposeManifest(milvusapp.DockerComposeManifest.Name, milvusapp.DockerComposeManifest.Content),
		dockeragentparams.WithPulumiDependsOn(utils.PulumiDependsOn(manager)),
	)
	dockerAgent, err := agent.NewDockerAgent(&awsEnv, host, manager, agentOptions...)
	if err != nil {
		return err
	}
	return dockerAgent.Export(ctx, env.DockerAgentOutput())
}

func vmOptionsFromEnvironment(e aws.Environment) []ec2.VMOption {
	var opts []ec2.VMOption
	osDesc := compos.DescriptorFromString(e.InfraOSDescriptor(), compos.UbuntuDefault)
	if img := e.InfraOSImageID(); img != "" {
		return append(opts, ec2.WithAMI(img, osDesc, osDesc.Architecture))
	}
	if e.InfraOSDescriptor() != "" {
		opts = append(opts, ec2.WithOS(osDesc))
	}
	if e.InfraOSImageIDUseLatest() {
		opts = append(opts, ec2.WithLatestAMI())
	}
	return opts
}

func agentOptionsFromEnvironment(e aws.Environment) []dockeragentparams.Option {
	var opts []dockeragentparams.Option
	if full := e.AgentFullImagePath(); full != "" {
		opts = append(opts, dockeragentparams.WithFullImagePath(full))
	} else if v := e.AgentVersion(); v != "" {
		opts = append(opts, dockeragentparams.WithImageTag(v))
	}
	if e.AgentJMX() {
		opts = append(opts, dockeragentparams.WithJMX())
	}
	if e.AgentFIPS() {
		opts = append(opts, dockeragentparams.WithFIPS())
	}
	return opts
}
