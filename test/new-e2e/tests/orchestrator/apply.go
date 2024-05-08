// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	_ "embed"
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	fakeintakeComp "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	resAws "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/ec2"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake"
)

//go:embed agent_values.yaml
var agentCustomValuesFmt string

// Apply creates a kind cluster, deploys the datadog agent, and installs various workloads for testing
func Apply(ctx *pulumi.Context) error {
	awsEnv, kindCluster, kindKubeProvider, err := createCluster(ctx)
	if err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	agentDependency, err := deployAgent(ctx, awsEnv, kindCluster, kindKubeProvider)
	if err != nil {
		return fmt.Errorf("failed to deploy agent to cluster: %w", err)
	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
		if _, err := redis.K8sAppDefinition(awsEnv, kindKubeProvider, "workload-redis", agentDependency); err != nil {
			return fmt.Errorf("failed to install redis: %w", err)
		}
	}

	return nil
}

func createCluster(ctx *pulumi.Context) (*resAws.Environment, *localKubernetes.Cluster, *kubernetes.Provider, error) {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return nil, nil, nil, err
	}

	vm, err := ec2.NewVM(awsEnv, "kind")
	if err != nil {
		return nil, nil, nil, err
	}
	if err := vm.Export(ctx, nil); err != nil {
		return nil, nil, nil, err
	}

	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, vm)
	if err != nil {
		return nil, nil, nil, err
	}

	kindCluster, err := localKubernetes.NewKindCluster(&awsEnv, vm, awsEnv.CommonNamer().ResourceName("kind"), "kind", awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return nil, nil, nil, err
	}
	if err := kindCluster.Export(ctx, nil); err != nil {
		return nil, nil, nil, err
	}

	// Export cluster’s properties
	ctx.Export("kubeconfig", kindCluster.KubeConfig)

	// Building Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.BoolPtr(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return &awsEnv, kindCluster, kindKubeProvider, nil
}

func deployAgent(ctx *pulumi.Context, awsEnv *resAws.Environment, cluster *localKubernetes.Cluster, kindKubeProvider *kubernetes.Provider) (pulumi.ResourceOption, error) {
	var agentDependency pulumi.ResourceOption

	var fakeIntake *fakeintakeComp.Fakeintake
	if awsEnv.GetCommonEnvironment().AgentUseFakeintake() {
		fakeIntakeOptions := []fakeintake.Option{}
		if awsEnv.GetCommonEnvironment().InfraShouldDeployFakeintakeWithLB() {
			fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithLoadBalancer())
		}

		var err error
		if fakeIntake, err = fakeintake.NewECSFargateInstance(*awsEnv, cluster.Name(), fakeIntakeOptions...); err != nil {
			return nil, err
		}
		if err := fakeIntake.Export(awsEnv.Ctx(), nil); err != nil {
			return nil, err
		}
	}

	clusterName := ctx.Stack()

	// Deploy the agent
	if awsEnv.AgentDeploy() {
		customValues := fmt.Sprintf(agentCustomValuesFmt, clusterName)
		helmComponent, err := agent.NewHelmInstallation(awsEnv, agent.HelmInstallationArgs{
			KubeProvider: kindKubeProvider,
			Namespace:    "datadog",
			ValuesYAML: pulumi.AssetOrArchiveArray{
				pulumi.NewStringAsset(customValues),
			},
			Fakeintake: fakeIntake,
		}, nil)
		if err != nil {
			return nil, err
		}

		ctx.Export("kube-cluster-name", pulumi.String(clusterName))
		ctx.Export("agent-linux-helm-install-name", helmComponent.LinuxHelmReleaseName)
		ctx.Export("agent-linux-helm-install-status", helmComponent.LinuxHelmReleaseStatus)

		agentDependency = utils.PulumiDependsOn(helmComponent)
	}

	// Deploy standalone dogstatsd
	if awsEnv.DogstatsdDeploy() {
		if _, err := dogstatsdstandalone.K8sAppDefinition(awsEnv, kindKubeProvider, "dogstatsd-standalone", fakeIntake, false, clusterName); err != nil {
			return nil, err
		}
	}

	return agentDependency, nil
}
