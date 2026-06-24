// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package aws_neuron provisions a single EC2 AWS Inferentia (inf2) instance running
// the Datadog Agent with the aws_neuron OpenMetricsV2 integration. AWS Neuron Monitor
// exposes a Prometheus endpoint on :8000 that the check scrapes, and a continuous
// Neuron inference workload drives real NeuronCore activity.
package aws_neuron

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	compos "github.com/DataDog/datadog-agent/test/e2e-framework/components/os"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	awsec2 "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// hostName is the remote host role name. The ec2 scenario exports the host
	// under the Pulumi key dd-Host-<hostName>, which the generated task reads via
	// REMOTE_HOSTNAME ("agent-host").
	hostName = "agent-host"

	// instanceType is an AWS Inferentia2 accelerated instance. Neuron Monitor only
	// emits real neuroncore/neuron_runtime/execution metrics on inf1/inf2/trn1/trn2
	// hardware with the Neuron driver + AWS Neuron Tools installed (capacity plan).
	instanceType = "inf2.xlarge"

	// dlamiOwner is the AWS account that publishes the Deep Learning AMIs.
	dlamiOwner = "898082745236"

	// dlamiNamePattern matches the Neuron Deep Learning AMI (Ubuntu 22.04), which
	// ships the Neuron driver, SDK, AWS Neuron Tools (neuron-monitor) and torch-neuronx.
	dlamiNamePattern = "Deep Learning AMI Neuron*Ubuntu 22.04*"
)

// neuronWorkload bootstraps Neuron Monitor's Prometheus exporter (:8000) and a
// continuous bounded Neuron inference loop. Embedded so it is layered into EC2 user data.
//
//go:embed workload/neuron_workload.sh
var neuronWorkload string

// checkConfig is the aws_neuron OpenMetricsV2 integration configuration. The check
// applies its own METRIC_MAP allowlist; the config only sets openmetrics_endpoint.
//
//go:embed config/aws_neuron.yaml
var checkConfig string

// Run is the scenario entry point registered as "aws/aws_neuron".
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

	params := ec2.GetParams(
		ec2.WithName(hostName),
		ec2.WithEC2InstanceOptions(
			ec2.WithAMI(ami, osDesc, osDesc.Architecture),
			ec2.WithInstanceType(instanceType),
			ec2.WithUserData(neuronWorkload),
		),
		ec2.WithAgentOptions(
			agentparams.WithIntegration("aws_neuron.d", checkConfig),
		),
		// Real E2E by default: no fakeintake unless explicitly opted in.
		ec2.WithoutFakeIntake(),
	)

	return ec2.Run(ctx, awsEnv, env, params)
}
