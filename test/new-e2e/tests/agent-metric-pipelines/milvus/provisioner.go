// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package milvus provides an E2E scenario that deploys Milvus, drives real
// traffic against it, runs a host Agent configured with the Milvus
// integration, and expects metrics to flow to a real Datadog intake (no
// fakeintake is provisioned).
package milvus

import (
	_ "embed"
	"strings"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/components"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
)

// Env is the typed environment for the Milvus E2E scenario. It deliberately
// does not include a FakeIntake: the Agent is configured to ship telemetry to
// the real Datadog intake using the API key from the runner configuration.
type Env struct {
	RemoteHost *components.RemoteHost
	Agent      *components.RemoteHostAgent
	Docker     *components.RemoteHostDocker
}

//go:embed testfixtures/docker-compose.milvus.yaml
var milvusComposeContent string

//go:embed testfixtures/milvus_integration.conf.yaml
var milvusIntegrationTemplate string

//go:embed testfixtures/traffic.py
var trafficScript string

// EnvProvisioner returns a Pulumi run function that provisions a single AWS EC2
// VM with Docker, deploys a Milvus standalone stack plus a traffic generator,
// and installs a host Agent with the Milvus integration enabled.
//
// The provided testID is tagged onto every metric the integration emits so the
// test can scope its Datadog API queries to a single run.
func EnvProvisioner(testID string) provisioners.PulumiEnvRunFunc[Env] {
	return func(ctx *pulumi.Context, env *Env) error {
		awsEnv, err := aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}

		// Amazon Linux ECS AMI ships Docker pre-installed, matching the
		// lighttpd example pattern in test/new-e2e/examples/.
		remoteHost, err := ec2.NewVM(awsEnv, "milvus", ec2.WithOS(os.AmazonLinuxECSDefault))
		if err != nil {
			return err
		}
		if err := remoteHost.Export(ctx, &env.RemoteHost.HostOutput); err != nil {
			return err
		}

		dockerManager, err := docker.NewAWSManager(&awsEnv, remoteHost)
		if err != nil {
			return err
		}
		if err := dockerManager.Export(ctx, &env.Docker.ManagerOutput); err != nil {
			return err
		}

		// Stage the traffic generator script on the VM so the compose can
		// mount it into the traffic container.
		tmpDirCmd, milvusDir, err := remoteHost.OS.FileManager().TempDirectory("milvus")
		if err != nil {
			return err
		}
		trafficPath := milvusDir + "/traffic.py"
		trafficCmd, err := remoteHost.OS.FileManager().CopyInlineFile(
			pulumi.String(trafficScript),
			trafficPath,
		)
		if err != nil {
			return err
		}

		composeCmd, err := dockerManager.ComposeStrUp(
			"milvus",
			[]docker.ComposeInlineManifest{{
				Name:    "milvus",
				Content: pulumi.String(milvusComposeContent),
			}},
			pulumi.StringMap{
				"DD_MILVUS_TRAFFIC_SCRIPT": pulumi.String(trafficPath),
			},
			pulumi.DependsOn([]pulumi.Resource{tmpDirCmd, trafficCmd}),
		)
		if err != nil {
			return err
		}

		// Stamp the unique test id into the integration config so the test
		// can filter metrics on `e2e_test_id:<testID>`.
		integrationConf := strings.ReplaceAll(
			milvusIntegrationTemplate,
			"E2E_TEST_ID_PLACEHOLDER",
			testID,
		)

		// Install the host Agent. We intentionally do NOT call
		// agentparams.WithFakeintake: omitting it makes the Agent forward
		// telemetry to the real Datadog intake using the API key configured
		// for the runner (DD_AGENT_API_KEY / e2e profile).
		hostAgent, err := agent.NewHostAgent(&awsEnv, remoteHost,
			agentparams.WithIntegration("milvus.d", integrationConf),
			agentparams.WithTags([]string{
				"env:e2e",
				"e2e_scenario:milvus",
				"e2e_test_id:" + testID,
			}),
			agentparams.WithPulumiResourceOptions(pulumi.DependsOn([]pulumi.Resource{composeCmd})),
		)
		if err != nil {
			return err
		}
		return hostAgent.Export(ctx, &env.Agent.HostAgentOutput)
	}
}
