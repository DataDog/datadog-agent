// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package redisdb provisions a single AWS EC2 VM running the Datadog Agent
// alongside a standalone Redis 7.2 OSS Docker container on localhost:6379.
// A background workload continuously exercises Redis so the bundled redisdb
// core check observes meaningful INFO / commandstats / slowlog / keyspace
// behaviour.
package redisdb

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	scenarioName = "redisdb"
	// redisImage pins the standalone Redis 7.2 OSS image used by the lab.
	redisImage = "redis:7.2"
)

//go:embed config/redisdb.yaml
var redisdbCheckConfig string

//go:embed config/workload.sh
var workloadScript string

// Run is the scenario entry point invoked via Pulumi.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewHost()

	// Single x86_64 Ubuntu VM sized per the capacity plan (t3.medium, 2 vCPU /
	// 4 GiB / 20 GiB).
	host, err := ec2.NewVM(awsEnv, scenarioName, ec2.WithInstanceType("t3.medium"))
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// Install Docker on the VM. Agent install is serialized after Docker to
	// avoid apt-lock contention (mirrors the stock ec2 scenario).
	dockerManager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}

	// Launch the standalone Redis 7.2 OSS container, bound to loopback:6379.
	redisRun, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("redis-run"),
		&command.Args{
			Create: pulumi.Sprintf(
				"docker rm -f redis-lab >/dev/null 2>&1 || true; "+
					"docker run -d --restart unless-stopped --name redis-lab "+
					"-p 127.0.0.1:6379:6379 %s redis-server --save '' --appendonly no",
				redisImage,
			),
			Delete: pulumi.String("docker rm -f redis-lab >/dev/null 2>&1 || true"),
		},
		utils.PulumiDependsOn(dockerManager),
		pulumi.DeleteBeforeReplace(true),
	)
	if err != nil {
		return err
	}

	// Wait for Redis to accept connections, then self-diagnose on failure.
	redisReady, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("redis-ready"),
		&command.Args{
			Create: pulumi.String(
				"for i in $(seq 1 60); do " +
					"if docker exec redis-lab redis-cli -h 127.0.0.1 -p 6379 ping 2>/dev/null | grep -q PONG; then " +
					"echo 'redis ready'; exit 0; fi; sleep 2; done; " +
					"echo 'redis did not become ready' >&2; " +
					"docker ps -a >&2; " +
					"docker logs redis-lab >&2 2>&1 || true; " +
					"exit 1",
			),
		},
		utils.PulumiDependsOn(redisRun),
	)
	if err != nil {
		return err
	}

	// Install the continuous workload driver and run it inside the Redis
	// container under a detached, self-restarting helper.
	workloadStart, err := host.OS.Runner().Command(
		awsEnv.Namer.ResourceName("redis-workload"),
		&command.Args{
			Create: pulumi.Sprintf(
				"cat > /tmp/redis-workload.sh <<'__WORKLOAD__'\n%s\n__WORKLOAD__\n"+
					"docker cp /tmp/redis-workload.sh redis-lab:/tmp/redis-workload.sh; "+
					"docker exec -d redis-lab sh -c 'while true; do sh /tmp/redis-workload.sh; sleep 1; done'; "+
					"echo 'workload started'",
				workloadScript,
			),
			Delete: pulumi.String(
				"docker exec redis-lab sh -c \"pkill -f redis-workload.sh\" >/dev/null 2>&1 || true",
			),
		},
		utils.PulumiDependsOn(redisReady),
		pulumi.DeleteBeforeReplace(true),
	)
	if err != nil {
		return err
	}

	// Install the Datadog Agent with the redisdb integration config.
	agentOptions := []agentparams.Option{
		agentparams.WithIntegration("redisdb.d", redisdbCheckConfig),
		agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
		// Serialize agent install behind docker (apt lock) and the workload.
		agentparams.WithPulumiResourceOptions(
			utils.PulumiDependsOn(dockerManager),
			utils.PulumiDependsOn(workloadStart),
		),
	}

	agentComp, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
	if err != nil {
		return err
	}
	if err := agentComp.Export(ctx, env.AgentOutput()); err != nil {
		return err
	}

	// This scenario does not provision fakeintake or the updater.
	env.DisableFakeIntake()
	env.DisableUpdater()

	return nil
}
