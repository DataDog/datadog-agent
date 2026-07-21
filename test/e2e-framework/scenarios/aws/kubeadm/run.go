// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kubeadm contains the Pulumi program for a single-node kubeadm cluster
// running directly on an EC2 VM with containerd, plus the Datadog Agent and the
// SBOM target workloads.
package kubeadm

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/sbomtargets"
	fakeintakeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
)

// Run is the entry point for the scenario when run via pulumi.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewKubernetes()
	return RunWithEnv(ctx, awsEnv, env, ParamsFromEnvironment(awsEnv))
}

// RunWithEnv deploys a kubeadm-on-EC2 environment using a provided env and params.
func RunWithEnv(ctx *pulumi.Context, awsEnv resAws.Environment, env outputs.KubernetesOutputs, params *RunParams) error {
	var err error
	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, params.Name, params.fakeintakeOptions...)
		if err != nil {
			return err
		}
		if err = fakeIntake.Export(ctx, env.FakeIntakeOutput()); err != nil {
			return err
		}

		if len(params.agentOptions) > 0 {
			newOpts := []kubernetesagentparams.Option{kubernetesagentparams.WithFakeintake(fakeIntake)}
			params.agentOptions = append(newOpts, params.agentOptions...)
		}
		params.vmOptions = append(params.vmOptions, ec2.WithPulumiResourceOptions(utils.PulumiDependsOn(fakeIntake)))
	} else {
		env.DisableFakeIntake()
	}

	host, err := ec2.NewVM(awsEnv, params.Name, params.vmOptions...)
	if err != nil {
		return err
	}

	cluster, err := kubeComp.NewKubeadmCluster(&awsEnv, host, params.Name, awsEnv.KubernetesVersion(), params.containerRuntime)
	if err != nil {
		return err
	}
	if err = cluster.Export(ctx, env.KubernetesClusterOutput()); err != nil {
		return err
	}

	// If InitOnly is set, return after creating the cluster.
	if awsEnv.InitOnly() {
		return nil
	}

	kubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            cluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	var dependsOnDDAgent pulumi.ResourceOption
	if len(params.agentOptions) > 0 {
		newOpts := []kubernetesagentparams.Option{
			kubernetesagentparams.WithClusterName(cluster.ClusterName),
			kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
		}
		params.agentOptions = append(newOpts, params.agentOptions...)
		agent, err := helm.NewKubernetesAgent(&awsEnv, params.Name, kubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		if err = agent.Export(ctx, env.KubernetesAgentOutput()); err != nil {
			return err
		}
		dependsOnDDAgent = utils.PulumiDependsOn(agent)
	} else {
		env.DisableAgent()
	}

	if params.deploySBOMWorkloads {
		var extraOpts []pulumi.ResourceOption
		if dependsOnDDAgent != nil {
			extraOpts = append(extraOpts, dependsOnDDAgent)
		}
		if _, err := sbomtargets.K8sAppDefinition(&awsEnv, kubeProvider, extraOpts...); err != nil {
			return err
		}
	}

	return nil
}
