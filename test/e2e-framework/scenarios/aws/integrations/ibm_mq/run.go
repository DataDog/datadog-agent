// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package ibm_mq provisions a single AWS EC2 VM (x86_64, sized from the capacity
// plan) that co-locates an IBM MQ Advanced for Developers 9.3 server, a continuous
// put/get load generator, and the Datadog Agent running the bundled ibm_mq check.
//
// Topology (single-vm):
//   - agent-host (m5.2xlarge, RHEL 8 x86_64): N queue managers (QM1..QMn) each
//     listening on a distinct TCP port from BasePort, each with QueuesPerQM local
//     queues and a DEV.ADMIN.SVRCONN channel; a systemd load service that puts/gets
//     messages so queue depth + enqueue/dequeue rates are non-zero; and the host
//     Datadog Agent with one ibm_mq instance per queue manager. vm.Export() emits
//     dd-Host-aws-agent-host (task role "aws-agent-host").
//
// ARCH CONSTRAINT: ibm_mq is in ARM_EXCLUSIONS (no arm64 build) and pymqi needs the
// IBM MQ C client libraries at /opt/mqm; both are satisfied by co-locating the MQ
// server on an amd64 host (ldconfig wires /opt/mqm/lib64 for the Agent service).
//
// This is the pre-exploration Phase-1 skeleton: provisioning lives inline in
// setup.go (embedded scripts). Exploration-hardened steps move into a reusable
// components/integration/ibm_mq package in the synthesis phase.
package ibm_mq

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// VMRun is the pulumi entry point registered in the integrations registry.
func VMRun(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}
	return Run(ctx, awsEnv, outputs.NewHost(), ParamsFromEnvironment(awsEnv))
}

// Run provisions the IBM MQ server + load generator and the Datadog Agent.
func Run(ctx *pulumi.Context, awsEnv aws.Environment, env outputs.HostOutputs, params *Params) error {
	// Force RHEL 8 x86_64 (AMD64) to match the target IBM MQ deployment environment.
	// RedHat is a first-class flavor with a framework-maintained AMI in the
	// default descriptor map; NewDescriptor(RedHat, "8") defaults to AMD64Arch. ibm_mq
	// has no arm64 build, so amd64 is mandatory. (AlmaLinux9 was rejected: it carries an
	// empty version and requires WithLatestAMI, absent from LinuxDescriptorsDefault.)
	instanceOptions := append([]ec2.VMOption{ec2.WithOS(compos.RedHat8)}, params.instanceOptions...)

	host, err := ec2.NewVM(awsEnv, params.Name, instanceOptions...)
	if err != nil {
		return err
	}
	// Export under dd-Host-aws-agent-host for the task surface (REMOTE_HOSTNAME=aws-agent-host).
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// Install + configure the IBM MQ server: N queue managers, listeners, channels,
	// and queues. Synchronous; the Agent depends on it.
	mqSetup, err := newMQSetup(&awsEnv, host, params)
	if err != nil {
		return err
	}

	// Continuous put/get load so queue depth + enqueue/dequeue metric families are
	// non-zero (PCF query cost scales with queue count, which is what this lab measures).
	load, err := newMQLoad(&awsEnv, host, params, utils.PulumiDependsOn(mqSetup))
	if err != nil {
		return err
	}

	// Readiness gate: block until every queue manager reports Running and its
	// listener port accepts connections, dumping diagnostics on failure, so the
	// Agent never scrapes a queue manager that is not up yet.
	ready, err := newMQReadiness(&awsEnv, host, params,
		utils.PulumiDependsOn(mqSetup, load))
	if err != nil {
		return err
	}

	// Real E2E lab: no fakeintake, no updater.
	env.DisableFakeIntake()
	env.DisableUpdater()

	if params.agentOptions == nil {
		env.DisableAgent()
		return nil
	}

	// Render the ibm_mq.d/conf.yaml (one instance per queue manager) and, when
	// profiling is enabled, a datadog.yaml overlay.
	checkConfig, err := renderCheckConfig(params)
	if err != nil {
		return err
	}

	agentOptions := make([]agentparams.Option, 0, len(params.agentOptions)+4)
	agentOptions = append(agentOptions, params.agentOptions...)
	agentOptions = append(agentOptions,
		agentparams.WithIntegration("ibm_mq.d", checkConfig),
		agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(ready)),
	)

	// Only override datadog.yaml when profiling is requested (an empty
	// WithAgentConfig would silently clobber the integration config).
	if params.IntegrationProfiling || params.InternalProfiling {
		agentConfig, err := renderAgentConfig(params)
		if err != nil {
			return err
		}
		agentOptions = append(agentOptions, agentparams.WithAgentConfig(agentConfig))
	}

	agentComp, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
	if err != nil {
		return err
	}
	if err := agentComp.Export(ctx, env.AgentOutput()); err != nil {
		return err
	}

	return nil
}
