// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package gensim

import (
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/command"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	kubeComp "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	resAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	helmresource "github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/fakeintake"

	pulumiKubernetes "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// Run creates an EC2+Kind cluster and deploys a gensim episode with custom Datadog Agent
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	// Get gensim-specific configuration
	cfg := config.New(ctx, "gensim")

	episodeName := cfg.Require("episodeName")         // e.g., "002_AWS_S3_Service_Disruption"
	episodeChartPath := cfg.Require("chartPath")      // Path to episode's Helm chart
	datadogValuesPath := cfg.Get("datadogValuesPath") // Path to datadog-values.yaml (optional)
	namespace := cfg.Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	// Create EC2 VM for the Kind cluster
	host, err := ec2.NewVM(awsEnv, "gensim")
	if err != nil {
		return err
	}

	// Install ECR credentials helper to allow pulling images from ECR
	installEcrCredsHelperCmd, err := ec2.InstallECRCredentialsHelper(awsEnv, host)
	if err != nil {
		return err
	}

	// Create Kind cluster on the EC2 VM
	kindCluster, err := kubeComp.NewKindCluster(&awsEnv, host, "gensim", awsEnv.KubernetesVersion(), utils.PulumiDependsOn(installEcrCredsHelperCmd))
	if err != nil {
		return err
	}

	if err := kindCluster.Export(ctx, nil); err != nil {
		return err
	}

	// Create Kubernetes provider from the Kind cluster kubeconfig
	kubeProvider, err := pulumiKubernetes.NewProvider(ctx, awsEnv.Namer.ResourceName("k8s-provider"), &pulumiKubernetes.ProviderArgs{
		EnableServerSideApply: pulumi.Bool(true),
		Kubeconfig:            kindCluster.KubeConfig,
	})
	if err != nil {
		return err
	}

	// Deploy custom Datadog Agent DaemonSet with observer capabilities
	// This replaces the episode's basic agent Deployment
	var dependsOnDDAgent pulumi.ResourceOption

	if awsEnv.AgentDeploy() {
		k8sAgentOptions := make([]kubernetesagentparams.Option, 0)

		// Deploy to the same namespace as the episode so services can reach the agent
		k8sAgentOptions = append(
			k8sAgentOptions,
			kubernetesagentparams.WithNamespace(namespace),
		)

		// Handle fakeintake if needed
		if awsEnv.AgentUseFakeintake() {
			fakeIntakeOptions := []fakeintake.Option{fakeintake.WithLoadBalancer()}
			if awsEnv.AgentUseDualShipping() {
				fakeIntakeOptions = append(fakeIntakeOptions, fakeintake.WithoutDDDevForwarding())
			}

			fakeIntake, err := fakeintake.NewECSFargateInstance(awsEnv, "gensim-fakeintake", fakeIntakeOptions...)
			if err != nil {
				return err
			}
			if err := fakeIntake.Export(ctx, nil); err != nil {
				return err
			}
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithFakeintake(fakeIntake))
		}

		// Load custom helm values from datadog-values.yaml if it exists
		// This includes observer configuration (DD_OBSERVER_* env vars)
		if datadogValuesPath != "" {
			valuesContent, err := os.ReadFile(datadogValuesPath)
			if err != nil {
				return err
			}
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithHelmValues(string(valuesContent)))
		}

		if awsEnv.AgentFullImagePath() != "" {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithAgentFullImagePath(awsEnv.AgentFullImagePath()))
		}

		if awsEnv.ClusterAgentFullImagePath() != "" {
			k8sAgentOptions = append(k8sAgentOptions, kubernetesagentparams.WithClusterAgentFullImagePath(awsEnv.ClusterAgentFullImagePath()))
		}

		k8sAgentComponent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent"), kubeProvider, k8sAgentOptions...)
		if err != nil {
			return err
		}

		if err := k8sAgentComponent.Export(ctx, nil); err != nil {
			return err
		}

		dependsOnDDAgent = utils.PulumiDependsOn(k8sAgentComponent)
	}

	// Deploy the episode's Helm chart
	// Helm release names must be lowercase alphanumeric with hyphens only, max 53 chars.
	sanitizedEpisodeName := strings.ToLower(strings.ReplaceAll(episodeName, "_", "-"))
	episodeReleaseName := fmt.Sprintf("gensim-%s", sanitizedEpisodeName)
	if len(episodeReleaseName) > 53 {
		episodeReleaseName = episodeReleaseName[:53]
	}

	episodeChart, err := helmresource.NewInstallation(&awsEnv, helmresource.InstallArgs{
		RepoURL:     "", // Local chart, no repo
		ChartName:   episodeChartPath,
		InstallName: episodeReleaseName,
		Namespace:   namespace,
		Values: pulumi.Map{
			"namespace": pulumi.String(namespace),
			"datadog": pulumi.Map{
				"apiKey": awsEnv.AgentAPIKey(),
				"appKey": awsEnv.AgentAPPKey(),
				"site":   pulumi.String(awsEnv.Site()),
				"env":    pulumi.String(fmt.Sprintf("gensim-%s", episodeName)),
			},
		},
	}, pulumi.Provider(kubeProvider), dependsOnDDAgent)

	if err != nil {
		return err
	}

	// The episode chart includes its own basic datadog-agent Deployment. Now that we deploy
	// the full DaemonSet-based agent, remove the episode's duplicate agent using
	// `docker exec` into the Kind control plane (which has kubectl pre-installed).
	if awsEnv.AgentDeploy() {
		_, err = host.OS.Runner().Command(
			awsEnv.Namer.ResourceName("delete-episode-agent"),
			&command.Args{
				Create: pulumi.Sprintf(
					"docker exec %s-control-plane kubectl delete deploy/datadog-agent svc/datadog-agent sa/datadog-agent --ignore-not-found=true -n %s",
					kindCluster.ClusterName, namespace,
				),
			},
			utils.PulumiDependsOn(episodeChart),
		)
		if err != nil {
			return err
		}
	}

	// Export episode information
	ctx.Export("episode-name", pulumi.String(episodeName))
	ctx.Export("episode-namespace", pulumi.String(namespace))
	ctx.Export("episode-release", episodeChart.Status.Name())

	return nil
}
