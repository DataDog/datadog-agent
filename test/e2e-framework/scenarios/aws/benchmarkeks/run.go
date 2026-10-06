// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package benchmarkeks provisions an EKS cluster partitioned into dedicated node pools
// so a baseline and a comparison version of the Agent can be compared side by side. What
// it compares is the resource footprint of the two Agents — CPU, memory, goroutines,
// uploaded profiles — while both monitor a strictly identical workload; the workload's
// own performance is not measured. KWOK simulates a large number of nodes/objects and the
// churn orchestrator continuously creates and deletes workloads.
package benchmarkeks

import (
	_ "embed"

	awsEks "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/eks"
	"github.com/pulumi/pulumi-eks/sdk/v4/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/churn"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/kwok"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenarioEks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
)

const nbNode = 3

//go:embed simple_check.py
var simpleCheckPy string

// Run is the entry point for the benchmarkeks scenario when run via Pulumi.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	cluster, err := scenarioEks.NewCluster(awsEnv, "eks")
	if err != nil {
		return err
	}

	if err := cluster.Export(ctx, nil); err != nil {
		return err
	}

	for _, ng := range []struct {
		role    string
		variant string
		nbNode  int
	}{
		{
			role:    "node-agent",
			variant: "baseline",
			nbNode:  nbNode,
		},
		{
			role:    "node-agent",
			variant: "comparison",
			nbNode:  nbNode,
		},
		{
			role:    "cluster-agent",
			variant: "baseline",
			nbNode:  1,
		},
		{
			role:    "cluster-agent",
			variant: "comparison",
			nbNode:  1,
		},
		{
			role:    "cluster-checks",
			variant: "baseline",
			nbNode:  1,
		},
		{
			role:    "cluster-checks",
			variant: "comparison",
			nbNode:  1,
		},
	} {
		// The dependency on the cluster component makes the node groups wait for the
		// CNI custom-networking setup (ENIConfig + aws-node patch) before joining: a
		// dependsOn on a component resource extends to all of its children.
		if _, err := eks.NewManagedNodeGroup(ctx, "ng-"+ng.role+"-"+ng.variant, &eks.ManagedNodeGroupArgs{
			Cluster:             cluster.Cluster.Core,
			InstanceTypes:       pulumi.ToStringArray([]string{awsEnv.DefaultInstanceType()}),
			ForceUpdateVersion:  pulumi.BoolPtr(true),
			NodeGroupNamePrefix: awsEnv.CommonNamer().DisplayName(37, pulumi.String("ng"), pulumi.String(ng.role), pulumi.String(ng.variant)),
			ScalingConfig: awsEks.NodeGroupScalingConfigArgs{
				DesiredSize: pulumi.Int(ng.nbNode),
				MaxSize:     pulumi.Int(ng.nbNode),
				MinSize:     pulumi.Int(ng.nbNode),
			},
			NodeRole: cluster.Cluster.InstanceRoles.Index(pulumi.Int(0)),
			Labels: pulumi.StringMap{
				"benchmark.datadoghq.com/role":    pulumi.String(ng.role),
				"benchmark.datadoghq.com/variant": pulumi.String(ng.variant),
			},
			Taints: awsEks.NodeGroupTaintArray{
				awsEks.NodeGroupTaintArgs{
					Key:    pulumi.String("benchmark.datadoghq.com/role"),
					Value:  pulumi.String(ng.role),
					Effect: pulumi.String("NO_SCHEDULE"),
				},
				awsEks.NodeGroupTaintArgs{
					Key:    pulumi.String("benchmark.datadoghq.com/variant"),
					Value:  pulumi.String(ng.variant),
					Effect: pulumi.String("NO_SCHEDULE"),
				},
			},
			RemoteAccess: awsEks.NodeGroupRemoteAccessArgs{
				Ec2SshKey:              pulumi.StringPtr(awsEnv.DefaultKeyPairName()),
				SourceSecurityGroupIds: pulumi.ToStringArray(awsEnv.EKSAllowedInboundSecurityGroups()),
			},
		}, awsEnv.WithProviders(config.ProviderAWS, config.ProviderEKS), utils.PulumiDependsOn(cluster)); err != nil {
			return err
		}
	}

	if _, err := kwok.K8sAppDefinition(&awsEnv, cluster.KubeProvider); err != nil {
		return err
	}

	// The Agent installations and the churn orchestrator both require the Agent API
	// key (via Helm and the churn Fargate sidecar), which is only configured when the
	// Agent is deployed. Skip them when the Agent is disabled (--no-install-agent).
	if !awsEnv.AgentDeploy() {
		return nil
	}

	var agentDeps []pulumi.Resource

	// The per-variant image keys live in the ddagent: config namespace and are specific
	// to this scenario, so they are read here rather than through the shared config.Env
	// interface — same pattern as scenarios/aws/integrations/{lustre,dell_powerflex}.
	// The invoke task maps its --baseline-* / --comparison-* flags onto them (see
	// tasks/e2e_framework/aws/benchmarkeks.py).
	for _, param := range []struct {
		variant               string
		agentImagePath        string
		clusterAgentImagePath string
		agentVersion          string
		clusterAgentVersion   string
		deployCRDs            bool
	}{
		{
			variant:               "baseline",
			agentImagePath:        awsEnv.AgentConfig.Get("baselineFullImagePath"),
			clusterAgentImagePath: awsEnv.AgentConfig.Get("baselineClusterAgentFullImagePath"),
			agentVersion:          awsEnv.AgentConfig.Get("baselineVersion"),
			clusterAgentVersion:   awsEnv.AgentConfig.Get("baselineClusterAgentVersion"),
			deployCRDs:            true,
		},
		{
			variant:               "comparison",
			agentImagePath:        awsEnv.AgentConfig.Get("comparisonFullImagePath"),
			clusterAgentImagePath: awsEnv.AgentConfig.Get("comparisonClusterAgentFullImagePath"),
			agentVersion:          awsEnv.AgentConfig.Get("comparisonVersion"),
			clusterAgentVersion:   awsEnv.AgentConfig.Get("comparisonClusterAgentVersion"),
			deployCRDs:            false,
		},
	} {
		kubernetesAgent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent", param.variant), cluster.KubeProvider,
			kubernetesagentparams.WithBaseName("dda-"+param.variant),
			kubernetesagentparams.WithNamespace("datadog-"+param.variant),
			kubernetesagentparams.WithClusterName(cluster.ClusterName),
			kubernetesagentparams.WithAgentFullImagePath(param.agentImagePath),
			kubernetesagentparams.WithClusterAgentFullImagePath(param.clusterAgentImagePath),
			kubernetesagentparams.WithAgentVersion(param.agentVersion),
			kubernetesagentparams.WithClusterAgentVersion(param.clusterAgentVersion),
			kubernetesagentparams.WithHelmValues(utils.YAMLMustMarshal(map[string]any{
				"datadog": map[string]any{
					"nodeLabelsAsTags": map[string]any{
						"benchmark.datadoghq.com/role":    "role",
						"benchmark.datadoghq.com/variant": "variant",
					},
					"podLabelsAsTags": map[string]any{
						"app": "app",
					},
					"env": []map[string]string{
						{
							"name":  "DD_INTERNAL_PROFILING_BLOCK_PROFILE_RATE",
							"value": "10000",
						},
						{
							"name":  "DD_INTERNAL_PROFILING_ENABLE_BLOCK_PROFILING",
							"value": "true",
						},
						{
							"name":  "DD_INTERNAL_PROFILING_ENABLE_GOROUTINE_STACKTRACES",
							"value": "true",
						},
						{
							"name":  "DD_INTERNAL_PROFILING_ENABLE_MUTEX_PROFILING",
							"value": "true",
						},
						{
							"name":  "DD_INTERNAL_PROFILING_ENABLED",
							"value": "true",
						},
						{
							"name":  "DD_INTERNAL_PROFILING_MUTEX_PROFILE_FRACTION",
							"value": "100",
						},
					},
					"checksd": map[string]string{
						"simple_check.py": simpleCheckPy,
					},
					"operator": map[string]any{
						"enabled": false,
					},
				},
				"agents": map[string]any{
					"nodeSelector": map[string]any{
						"benchmark.datadoghq.com/variant": param.variant,
					},
					// The node Agent runs as a DaemonSet on every node of its variant
					// (all role pools), so it tolerates any role taint.
					"tolerations": benchmarkTolerations(map[string]any{
						"key":      "benchmark.datadoghq.com/role",
						"operator": "Exists",
						"effect":   "NoSchedule",
					}, param.variant),
				},
				"clusterAgent": map[string]any{
					"replicas": 1,
					"nodeSelector": map[string]any{
						"benchmark.datadoghq.com/role":    "cluster-agent",
						"benchmark.datadoghq.com/variant": param.variant,
					},
					"tolerations": benchmarkTolerations(map[string]any{
						"key":      "benchmark.datadoghq.com/role",
						"operator": "Equal",
						"value":    "cluster-agent",
						"effect":   "NoSchedule",
					}, param.variant),
					"metricsProvider": map[string]any{
						"registerAPIService": false,
					},
					"admissionController": map[string]any{
						"agentSidecarInjection": map[string]any{
							"clusterAgentCommunicationEnabled": false,
						},
					},
				},
				"clusterChecksRunner": map[string]any{
					"replicas": 1,
					"nodeSelector": map[string]any{
						"benchmark.datadoghq.com/role":    "cluster-checks",
						"benchmark.datadoghq.com/variant": param.variant,
					},
					"tolerations": benchmarkTolerations(map[string]any{
						"key":      "benchmark.datadoghq.com/role",
						"operator": "Equal",
						"value":    "cluster-checks",
						"effect":   "NoSchedule",
					}, param.variant),
				},
				// The datadog-crds subchart renders cluster-scoped CRDs with Helm
				// ownership annotations, so only one of the two releases may own them.
				// This list must cover every CRD the subchart enables by default (see
				// charts/datadog/values.yaml in DataDog/helm-charts), otherwise the
				// second release fails with a "meta.helm.sh/release-name" error.
				"datadog-crds": map[string]any{
					"crds": map[string]any{
						"datadogMetrics":                      param.deployCRDs,
						"datadogPodAutoscalers":               param.deployCRDs,
						"datadogPodAutoscalerClusterProfiles": param.deployCRDs,
						"datadogInstrumentations":             param.deployCRDs,
					},
				},
			})),
		)
		if err != nil {
			return err
		}
		agentDeps = append(agentDeps, kubernetesAgent)
	}

	// The churn pods are labeled for Fargate sidecar injection, so they must wait for
	// the Agent admission controllers to be ready, otherwise they would be admitted
	// without the Datadog sidecar.
	if _, err := churn.K8sAppDefinition(&awsEnv, cluster.KubeProvider, utils.PulumiDependsOn(agentDeps...)); err != nil {
		return err
	}

	return nil
}

// benchmarkTolerations builds the toleration list for a benchmark workload: the
// caller-provided toleration for the role-pool taint, plus the shared per-variant
// taint toleration that keeps baseline and comparison workloads isolated.
func benchmarkTolerations(roleToleration map[string]any, variant string) []any {
	return []any{
		roleToleration,
		map[string]any{
			"key":      "benchmark.datadoghq.com/variant",
			"operator": "Equal",
			"value":    variant,
			"effect":   "NoSchedule",
		},
	}
}
