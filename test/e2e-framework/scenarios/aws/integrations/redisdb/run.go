// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package redisdb provisions a single AWS EC2 VM (sized from the capacity plan via Params)
// running the Datadog Agent alongside a standalone Redis 7.2 OSS Docker container on
// localhost:6379. A background workload continuously exercises Redis so the bundled redisdb
// core check observes meaningful INFO / commandstats / slowlog / keyspace behaviour.
package redisdb

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	redisdbcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/redisdb"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed config/redisdb.yaml
var redisdbCheckConfig string

// Run provisions the redisdb lab: an EC2 host with Docker, a Redis 7.2 + workload Compose
// stack, and the host-installed Datadog Agent configured with the redisdb check pointed at
// localhost:6379.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	host, err := ec2.NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// Install Docker on the VM.
	manager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}

	// Redis 7.2 OSS server (published on host loopback:6379) plus the continuous
	// workload generator, via Compose. The server's healthcheck gates the
	// workload container and the Agent install below.
	redisManifest, redisAssets, err := redisdbcomp.NewDockerCompose(manager)
	if err != nil {
		return err
	}
	composeDeps := make([]pulumi.ResourceOption, 0, len(redisAssets)+1)
	composeDeps = append(composeDeps, utils.PulumiDependsOn(manager))
	for _, asset := range redisAssets {
		composeDeps = append(composeDeps, utils.PulumiDependsOn(asset))
	}
	redisStack, err := manager.ComposeStrUp("redisdb", []docker.ComposeInlineManifest{redisManifest}, nil, composeDeps...)
	if err != nil {
		return err
	}

	// This scenario does not provision fakeintake or the updater.
	env.DisableFakeIntake()
	env.DisableUpdater()

	if params.agentOptions == nil {
		env.DisableAgent()
		return nil
	}

	// Install the Datadog Agent with the redisdb integration config. The check
	// waits for the Redis stack so redisdb.can_connect is healthy on first run.
	agentOptions := make([]agentparams.Option, 0, len(params.agentOptions)+3)
	agentOptions = append(agentOptions, params.agentOptions...)
	agentOptions = append(agentOptions,
		agentparams.WithIntegration("redisdb.d", redisdbCheckConfig),
		agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(redisStack)),
	)

	agentComp, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
	if err != nil {
		return err
	}
	if err := agentComp.Export(ctx, env.AgentOutput()); err != nil {
		return err
	}

	return nil
}

// VMRun is the pulumi entry point for the redisdb scenario.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	env := outputs.NewHost()
	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
