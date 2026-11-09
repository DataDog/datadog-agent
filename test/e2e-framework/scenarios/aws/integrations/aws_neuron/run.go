// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package aws_neuron provisions a single AWS Inferentia (inf2) EC2 instance running the Datadog
// Agent with the aws_neuron OpenMetricsV2 integration. AWS Neuron Monitor exposes a Prometheus
// endpoint on :8000 that the check scrapes, and a continuous Neuron inference workload drives real
// NeuronCore activity. The workload runs as a Pulumi command that the Agent install depends on, so
// the check never scrapes a cold endpoint.
package aws_neuron

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	awsec2 "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// hostName is the Agent host role. The ec2 scenario exports the host under the Pulumi key
	// dd-Host-aws-<hostName>, which the generated task reads as role "aws-agent-host".
	hostName = "agent-host"

	// instanceType is an AWS Inferentia2 accelerated instance. Neuron Monitor only emits real
	// neuroncore/neuron_runtime/execution metrics on inf1/inf2/trn1/trn2 hardware with the Neuron
	// driver + AWS Neuron Tools installed, so this family is pinned (special-hardware lab).
	instanceType = "inf2.xlarge"

	// dlamiOwner is the AWS account that publishes the Deep Learning AMIs.
	dlamiOwner = "898082745236"

	// dlamiNamePattern matches the Neuron Deep Learning AMI (Ubuntu 22.04), which ships the Neuron
	// driver, SDK, AWS Neuron Tools (neuron-monitor) and torch-neuronx.
	dlamiNamePattern = "Deep Learning AMI Neuron*Ubuntu 22.04*"

	// labAssetDir is where the workload script is staged on the host.
	labAssetDir = "/opt/datadog-neuron-lab"
)

// neuronWorkload starts Neuron Monitor's Prometheus exporter (:8000) plus a continuous Neuron
// inference loop as systemd units, then gates on the exporter being reachable. Embedded so it can
// be staged on the host and run as a Pulumi command.
//
//go:embed workload/neuron_workload.sh
var neuronWorkload string

// checkConfig is the aws_neuron OpenMetricsV2 integration configuration. The check applies its own
// METRIC_MAP allowlist; the config only sets openmetrics_endpoint.
//
//go:embed config/aws_neuron.yaml
var checkConfig string

// Run is the scenario entry point registered as "aws/integrations/aws_neuron".
func Run(ctx *pulumi.Context) error {
	awsEnv, err := aws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Resolve the Neuron DLAMI via the AMI search helper rather than a literal AMI id.
	osDesc := compos.Ubuntu2204
	ami, err := awsec2.SearchAMI(awsEnv, dlamiOwner, dlamiNamePattern, string(osDesc.Architecture))
	if err != nil {
		return fmt.Errorf("unable to resolve AWS Neuron DLAMI (owner=%s, name=%q): %w", dlamiOwner, dlamiNamePattern, err)
	}

	env := outputs.NewHost()

	host, err := ec2.NewVM(awsEnv, hostName,
		ec2.WithAMI(ami, osDesc, osDesc.Architecture),
		ec2.WithInstanceType(instanceType),
	)
	if err != nil {
		return err
	}
	if err := host.Export(ctx, env.RemoteHostOutput()); err != nil {
		return err
	}

	// Stage the workload script and run it before the Agent installs. The script starts the
	// Neuron Monitor exporter and inference load as systemd units and exits 0 only once the
	// Prometheus endpoint on :8000 answers. The Agent install depends on this command, so it is
	// the readiness gate: the aws_neuron check never scrapes a cold endpoint, and a failed
	// bring-up fails create with diagnostics instead of leaving an idle exporter.
	fm := host.OS.FileManager()
	mkdir, err := fm.CreateDirectory(labAssetDir, true)
	if err != nil {
		return err
	}
	scriptPath := labAssetDir + "/neuron-workload.sh"
	scriptFile, err := fm.CopyInlineFile(pulumi.String(neuronWorkload), scriptPath, pulumi.DependsOn([]pulumi.Resource{mkdir}))
	if err != nil {
		return err
	}
	workloadReady, err := host.OS.Runner().Command(
		awsEnv.CommonNamer().ResourceName("neuron-workload"),
		&command.Args{
			Create: pulumi.String("sudo bash " + scriptPath),
		},
		utils.PulumiDependsOn(scriptFile),
	)
	if err != nil {
		return err
	}

	agentOptions := []agentparams.Option{
		agentparams.WithIntegration("aws_neuron.d", checkConfig),
		agentparams.WithTags([]string{fmt.Sprintf("stackid:%s", ctx.Stack())}),
		agentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(workloadReady)),
	}
	agentComp, err := agent.NewHostAgent(&awsEnv, host, agentOptions...)
	if err != nil {
		return err
	}
	if err := agentComp.Export(ctx, env.AgentOutput()); err != nil {
		return err
	}

	env.DisableFakeIntake()
	env.DisableUpdater()
	return nil
}
