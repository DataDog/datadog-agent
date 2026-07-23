// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kafka provisions a single EC2 VM running a KRaft-mode Kafka broker in
// Docker Compose plus the host-resident Datadog Agent. JMXFetch on the host
// connects to the broker's JMX port (localhost:9999) to collect the bundled
// "kafka" JMX integration metrics.
package kafka

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/docker"
	kafkacomp "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/kafka"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	fakeintakescenario "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

//go:embed config/kafka.yaml
var kafkaCheckConfig string

// Run provisions the Kafka lab environment.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	host, err := ec2.NewVM(awsEnv, params.Name, params.instanceOptions...)
	if err != nil {
		return err
	}
	if err = host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// JMXFetch needs a JRE on the host to run the JMX-based "kafka" check; the
	// Agent package does not bundle one. Install a headless JRE before the Agent.
	jreReady, err := host.OS.Runner().Command(
		awsEnv.CommonNamer().ResourceName("kafka-install-jre"),
		&command.Args{
			Create: pulumi.String("sudo apt-get update -qq && sudo DEBIAN_FRONTEND=noninteractive apt-get install -y -qq default-jre-headless"),
		},
	)
	if err != nil {
		return err
	}

	// Install Docker + docker-compose on the host (ECR creds helper included).
	dockerManager, err := docker.NewAWSManager(&awsEnv, host)
	if err != nil {
		return err
	}

	// Bring up the Kafka broker + seed/load workload via docker-compose.
	composeUp, err := dockerManager.ComposeStrUp(
		"kafka",
		[]docker.ComposeInlineManifest{kafkacomp.DockerComposeManifest},
		pulumi.StringMap{},
	)
	if err != nil {
		return err
	}

	// Readiness gate: wait for the broker JMX port to accept connections before
	// installing the Agent. On failure, dump diagnostics so an opaque
	// connection-refused turns into a legible root cause.
	jmxReady, err := host.OS.Runner().Command(
		awsEnv.CommonNamer().ResourceName("kafka-jmx-ready"),
		&command.Args{
			Create: pulumi.String(`bash <<'EOF'
set -uo pipefail
for i in $(seq 1 60); do
  if (exec 3<>/dev/tcp/127.0.0.1/9999) 2>/dev/null; then
    exec 3>&- 3<&-
    echo "Kafka JMX port 9999 is accepting connections."
    exit 0
  fi
  sleep 5
done
echo "ERROR: Kafka JMX port 9999 did not become ready in time." >&2
echo "--- docker ps ---" >&2
sudo docker ps -a >&2 || true
echo "--- kafka-lab-broker logs (tail) ---" >&2
sudo docker logs --tail 200 kafka-lab-broker >&2 || true
echo "--- kafka-lab-init logs (tail) ---" >&2
sudo docker logs --tail 100 kafka-lab-init >&2 || true
echo "--- listening sockets ---" >&2
sudo ss -ltnp >&2 || sudo netstat -ltnp >&2 || true
exit 1
EOF`),
		},
		utils.PulumiDependsOn(composeUp),
	)
	if err != nil {
		return err
	}

	// FakeIntake is opt-in only.
	if params.fakeintakeOptions != nil {
		fakeIntake, err := fakeintakescenario.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, env.FakeIntakeOutput()); err != nil {
			return err
		}
		if params.agentOptions != nil {
			params.agentOptions = append([]agentparams.Option{agentparams.WithFakeintake(fakeIntake)}, params.agentOptions...)
		}
	} else {
		env.DisableFakeIntake()
	}
	env.DisableUpdater()

	// Install the host Agent with the bundled JMX "kafka" integration. The
	// Agent install depends on the JMX readiness gate (and therefore the
	// running broker) so JMXFetch can connect on first collection.
	if params.agentOptions != nil {
		agentOptions := append(params.agentOptions,
			agentparams.WithIntegration("kafka.d", kafkaCheckConfig),
			agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
			agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(jmxReady, jreReady)),
		)
		agentComp, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
		if err != nil {
			return err
		}
		if err = agentComp.Export(ctx, env.AgentOutput()); err != nil {
			return err
		}
		env.SetAgentClientOptions()
	} else {
		env.DisableAgent()
	}

	return nil
}

// VMRun is the pulumi entry point for the scenario.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	env := outputs.NewHost()
	return Run(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}
