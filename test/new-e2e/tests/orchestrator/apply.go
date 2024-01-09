// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestrator

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/agent"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps/redis"
	dogstatsdstandalone "github.com/DataDog/test-infra-definitions/components/datadog/dogstatsd-standalone"
	ddfakeintake "github.com/DataDog/test-infra-definitions/components/datadog/fakeintake"
	localKubernetes "github.com/DataDog/test-infra-definitions/components/kubernetes"
	awsResources "github.com/DataDog/test-infra-definitions/resources/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/fakeintake/fakeintakeparams"
	"github.com/DataDog/test-infra-definitions/scenarios/aws/vm/ec2vm"

	"github.com/pulumi/pulumi-kubernetes/sdk/v3/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// Apply creates a kind cluster, deploys the datadog agent, and installs various workloads for testing
func Apply(ctx *pulumi.Context) error {
	awsEnv, kindKubeProvider, err := createCluster(ctx)
	if err != nil {
		return fmt.Errorf("failed to create kind cluster: %w", err)
	}

	agentDependency, err := deployAgent(ctx, awsEnv, kindKubeProvider)
	if err != nil {
		return fmt.Errorf("failed to deploy agent to cluster: %w", err)
	}

	// Deploy testing workload
	if awsEnv.TestingWorkloadDeploy() {
		if _, err := redis.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "workload-redis", agentDependency); err != nil {
			return fmt.Errorf("failed to install redis: %w", err)
		}
	}

	return nil
}

func createCluster(ctx *pulumi.Context) (*awsResources.Environment, *kubernetes.Provider, error) {
	vm, err := ec2vm.NewUnixEc2VM(ctx)
	if err != nil {
		return nil, nil, err
	}
	awsEnv := vm.Infra.GetAwsEnvironment()

	kubeConfigCommand, kubeConfig, err := localKubernetes.NewKindCluster(vm.UnixVM, awsEnv.CommonNamer.ResourceName("kind"), "amd64", awsEnv.KubernetesVersion())
	if err != nil {
		return nil, nil, err
	}

	// Export cluster’s properties
	ctx.Export("kubeconfig", kubeConfig)

	// Building Kubernetes provider
	kindKubeProvider, err := kubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &kubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.BoolPtr(true),
		Kubeconfig:            kubeConfig,
	}, utils.PulumiDependsOn(kubeConfigCommand))
	if err != nil {
		return nil, nil, err
	}
	return &awsEnv, kindKubeProvider, nil
}

const agentCustomValuesFmt = `
datadog:
  kubelet:
    tlsVerify: false
  clusterName: "%s"
  orchestratorExplorer:
    customResources:
    - datadoghq.com/v1alpha1/datadogmetrics
agents:
  useHostNetwork: true

clusterAgent:
  enabled: true
  confd:
    orchestrator.yaml: |-
      init_config:
      instances:
        - collectors:
          - pods
          - nodes
          - deployments
          - customresourcedefinitions
          crd_collectors:
          - datadoghq.com/v1alpha1/datadogmetrics
`

func deployAgent(ctx *pulumi.Context, awsEnv *awsResources.Environment, kindKubeProvider *kubernetes.Provider) (pulumi.ResourceOption, error) {
	var agentDependency pulumi.ResourceOption

	var fakeIntake *ddfakeintake.ConnectionExporter
	var err error
	if awsEnv.GetCommonEnvironment().AgentUseFakeintake() {
		if fakeIntake, err = aws.NewEcsFakeintake(*awsEnv, fakeintakeparams.WithLoadBalancer()); err != nil {
			return nil, err
		}
	}

	clusterName := ctx.Stack()

	// Deploy the agent
	if awsEnv.AgentDeploy() {
		customValues := fmt.Sprintf(agentCustomValuesFmt, clusterName)
		helmComponent, err := agent.NewHelmInstallation(*awsEnv.CommonEnvironment, agent.HelmInstallationArgs{
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
		if _, err := dogstatsdstandalone.K8sAppDefinition(*awsEnv.CommonEnvironment, kindKubeProvider, "dogstatsd-standalone", fakeIntake, false, clusterName); err != nil {
			return nil, err
		}
	}

	return agentDependency, nil
}
