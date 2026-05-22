// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package agenthealth contains E2E tests for the agent health reporting functionality.
package agenthealth

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
)

// baseEC2Env holds the three components shared by all single-host health platform
// test environments.
type baseEC2Env struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Fakeintake *components.FakeIntake
}

// newBaseEC2Env provisions a single EC2 VM, a FakeIntake on ECS Fargate, and a
// Datadog agent with the given extra agentparams options. It populates env and
// returns any Pulumi error.
func newBaseEC2Env(
	ctx *pulumi.Context,
	env *baseEC2Env,
	vmName string,
	agentOptions ...func(*agentparams.Params) error,
) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	remoteHost, err := ec2.NewVM(awsEnv, vmName)
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

	hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
		append([]func(*agentparams.Params) error{agentparams.WithFakeintake(fi)}, agentOptions...)...,
	)
	if err != nil {
		return err
	}
	return hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
}
