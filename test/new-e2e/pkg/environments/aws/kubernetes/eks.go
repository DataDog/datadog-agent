// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package awskubernetes contains the provisioner for the Kubernetes based environments
package awskubernetes

import (
	"context"
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent/helm"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	"github.com/DataDog/test-infra-definitions/components/datadog/kubernetesagentparams"
	"github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/eks"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/e2e"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/utils/optional"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func eksDiagnoseFunc(ctx context.Context, stackName string) (string, error) {
	dumpResult, err := dumpEKSClusterState(ctx, stackName)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Dumping EKS cluster state:\n%s", dumpResult), nil
}

// EKSProvisioner creates a new provisioner
func EKSProvisioner(opts ...ProvisionerOption) e2e.TypedProvisioner[environments.Kubernetes] {
	// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
	// and it's easy to forget about it, leading to hard to debug issues.
	params := newProvisionerParams()
	_ = optional.ApplyOptions(params, opts)

	provisioner := e2e.NewTypedPulumiProvisioner(provisionerBaseID+params.name, func(ctx *pulumi.Context, env *environments.Kubernetes) error {
		// We ALWAYS need to make a deep copy of `params`, as the provisioner can be called multiple times.
		// and it's easy to forget about it, leading to hard to debug issues.
		params := newProvisionerParams()
		_ = optional.ApplyOptions(params, opts)

		return EKSRunFunc(ctx, env, params)
	}, params.extraConfigParams)

	provisioner.SetDiagnoseFunc(eksDiagnoseFunc)

	return provisioner
}

// EKSRunFunc deploys a EKS environment given a pulumi.Context
func EKSRunFunc(ctx *pulumi.Context, env *environments.Kubernetes, params *ProvisionerParams) error {
	var awsEnv aws.Environment
	var err error
	if params.awsEnv != nil {
		awsEnv = *params.awsEnv
	} else {
		awsEnv, err = aws.NewEnvironment(ctx)
		if err != nil {
			return err
		}
	}

	cluster, err := eks.NewCluster(awsEnv, params.name, params.eksOptions...)
	if err != nil {
		return err
	}

	if err := cluster.Export(ctx, &env.KubernetesCluster.ClusterOutput); err != nil {
		return err
	}

	if awsEnv.InitOnly() {
		return nil
	}

	var fakeIntake *fakeintakeComp.Fakeintake
	if params.fakeintakeOptions != nil {
		fakeIntakeOptions := []fakeintake.Option{
			fakeintake.WithCPU(1024),
			fakeintake.WithMemory(6144),
		}
		if awsEnv.GetCommonEnvironment().InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}

		if fakeIntake, err = fakeintake.NewECSFargateInstance(awsEnv, "ecs", fakeIntakeOptions...); err != nil {
			return err
		}
		if err := fakeIntake.Export(awsEnv.Ctx(), &env.FakeIntake.FakeintakeOutput); err != nil {
			return err
		}
	} else {
		env.FakeIntake = nil
	}

	// Deploy the agent
	dependsOnSetup := utils.PulumiDependsOn(cluster)
	if params.agentOptions != nil {
		params.agentOptions = append(params.agentOptions, kubernetesagentparams.WithPulumiResourceOptions(dependsOnSetup), kubernetesagentparams.WithFakeintake(fakeIntake))
		kubernetesAgent, err := helm.NewKubernetesAgent(&awsEnv, "eks", cluster.KubeProvider, params.agentOptions...)
		if err != nil {
			return err
		}
		err = kubernetesAgent.Export(ctx, &env.Agent.KubernetesAgentOutput)
		if err != nil {
			return err
		}
	} else {
		env.Agent = nil
	}

	// Deploy standalone dogstatsd
	if params.deployDogstatsd {
		if _, err := dogstatsdstandalone.K8sAppDefinition(&awsEnv, cluster.KubeProvider, "dogstatsd-standalone", fakeIntake, true, ""); err != nil {
			return err
		}
	}

	// Deploy workloads
	for _, appFunc := range params.workloadAppFuncs {
		_, err := appFunc(&awsEnv, cluster.KubeProvider)
		if err != nil {
			return err
		}
	}
	return nil
}
