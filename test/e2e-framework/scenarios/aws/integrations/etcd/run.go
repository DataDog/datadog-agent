// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package etcd provisions a single EC2 Docker host running an etcd 3.5 container
// (with a continuous v3 API load generator) alongside the Docker Datadog Agent
// and a FakeIntake. The Agent schedules the `etcd` integration against the etcd
// container's Prometheus endpoint via Docker autodiscovery labels.
package etcd

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/dockeragentparams"
	etcdapp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/etcd"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	hostName = "agent-host"
	// instanceType matches the capacity plan selection (t3.large: 2 vCPU / 8 GiB).
	instanceType = "t3.large"
)

// Run is the scenario entry point registered in the scenario registry.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewDockerHost()

	params := ec2docker.GetParams(
		ec2docker.WithName(hostName),
		ec2docker.WithEC2VMOptions(ec2.WithInstanceType(instanceType)),
		ec2docker.WithFakeIntakeOptions([]fakeintake.Option{}...),
		ec2docker.WithAgentOptions(
			dockeragentparams.WithExtraComposeManifest(
				etcdapp.DockerComposeManifest.Name,
				etcdapp.DockerComposeManifest.Content,
			),
		),
	)

	return ec2docker.Run(ctx, awsEnv, env, params)
}
