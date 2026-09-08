// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package datasecurity contains e2e tests for the packaged Data Security shared-library check.
package datasecurity

import (
	_ "embed"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	pgcomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/postgres"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

//go:embed fixtures/datadog-agent.yaml
var agentConfig string

//go:embed fixtures/datasecurity.yaml
var datasecurityConfig string

func postgresScanProvisioner() provisioners.PulumiEnvRunFunc[postgresScanEnv] {
	return func(ctx *pulumi.Context, env *postgresScanEnv) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		host, err := ec2.NewVM(awsEnv, "agent-host", ec2.WithInternetAccess())
		if err != nil {
			return err
		}
		if err := host.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		manager, err := docker.NewAWSManager(&awsEnv, host)
		if err != nil {
			return err
		}
		if err := manager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		pgManifest, pgAssets, err := pgcomp.NewDockerCompose(manager)
		if err != nil {
			return err
		}
		composeDeps := make([]pulumi.ResourceOption, 0, len(pgAssets)+1)
		composeDeps = append(composeDeps, utils.PulumiDependsOn(manager))
		for _, asset := range pgAssets {
			composeDeps = append(composeDeps, utils.PulumiDependsOn(asset))
		}
		pgStack, err := manager.ComposeStrUp("postgres", []docker.ComposeInlineManifest{pgManifest}, pulumi.StringMap{}, composeDeps...)
		if err != nil {
			return err
		}

		agentComp, err := agent.NewHostAgent(&awsEnv, host,
			agentparams.WithAgentConfig(agentConfig),
			agentparams.WithIntegration("datasecurity.d", datasecurityConfig),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(pgStack)),
		)
		if err != nil {
			return err
		}
		return agentComp.Export(ctx, &env.Agent.HostAgentOutput)
	}
}
