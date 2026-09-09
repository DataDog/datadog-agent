// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package postgres

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	pgcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/postgres"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// postgresCheckConfig is the conf.d/postgres.d/conf.yaml content deployed to the
// host Agent. It connects the check to the Dockerized PostgreSQL server.
//
//go:embed config/postgres.yaml
var postgresCheckConfig string

// Run provisions the aws/postgres environment: an EC2 host with Docker, a
// PostgreSQL 16 + workload Compose stack, and the host-installed Datadog Agent
// configured with the postgres integration pointed at localhost:5432.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	// Host VM. Named "agent-host"; exported as dd-Host-aws-agent-host.
	vmOptions := append([]ec2.VMOption{ec2.WithInstanceType(params.instanceType)}, params.vmOptions...)
	host, err := ec2.NewVM(awsEnv, params.Name, vmOptions...)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// Docker engine on the host.
	manager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}

	// PostgreSQL 16 server + continuous workload generator via Compose.
	pgManifest, pgAssets, err := pgcomp.NewDockerCompose(manager)
	if err != nil {
		return err
	}
	composeDeps := make([]pulumi.ResourceOption, 0, len(pgAssets)+1)
	composeDeps = append(composeDeps, utils.PulumiDependsOn(manager))
	for _, asset := range pgAssets {
		composeDeps = append(composeDeps, utils.PulumiDependsOn(asset))
	}
	pgStack, err := manager.ComposeStrUp("postgres", []docker.ComposeInlineManifest{pgManifest}, nil, composeDeps...)
	if err != nil {
		return err
	}

	// Mark optional outputs not used by this scenario.
	env.DisableFakeIntake()
	env.DisableUpdater()

	if !params.installAgent {
		env.DisableAgent()
		return nil
	}

	// Host Agent with the postgres integration. The check waits for the
	// PostgreSQL stack (and Docker) before starting so postgres.can_connect is
	// healthy on first run.
	agentOptions := make([]agentparams.Option, 0, len(params.agentOptions)+3)
	agentOptions = append(agentOptions, params.agentOptions...)
	agentOptions = append(agentOptions,
		agentparams.WithIntegration("postgres.d", postgresCheckConfig),
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(pgStack)),
		agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
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

// VMRun is the pulumi entry point for the aws/postgres scenario.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	env := outputs.NewHost()
	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
