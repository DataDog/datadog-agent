// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package benchmarkeks provisions an EKS cluster partitioned into node pools so a baseline
// and a comparison version of the Agent can be compared side by side: each of the two Agent
// installations, of variants a and b, has node pools of its own for its node Agents, its
// Cluster Agent and its cluster checks runner. What it compares is the resource footprint of
// the two Agents — CPU, memory, goroutines, uploaded profiles — while both monitor a strictly
// identical workload; the workload's own performance is not measured. KWOK simulates a large
// number of nodes/objects and the churn orchestrator continuously creates and deletes
// workloads.
package benchmarkeks

import (
	_ "embed"
	"maps"

	awsEks "github.com/pulumi/pulumi-aws/sdk/v7/go/aws/eks"
	"github.com/pulumi/pulumi-eks/sdk/v4/go/eks"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/churn"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/kwok"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	scenarioEks "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"
)

const (
	// RoleLabel is the label, and the taint key, of the nodes of a node pool, set to the
	// role of the pool.
	RoleLabel = "benchmark.datadoghq.com/role"
	// VariantLabel is the label, and the taint key, of the nodes of a node pool, set to the
	// variant of the Agent installation the pool belongs to.
	VariantLabel = "benchmark.datadoghq.com/variant"

	// RoleNodeAgent is the role of the node pools running the benchmark workload.
	RoleNodeAgent = "node-agent"
	// RoleClusterAgent is the role of the node pools dedicated to the Cluster Agent.
	RoleClusterAgent = "cluster-agent"
	// RoleClusterChecks is the role of the node pools dedicated to the cluster checks runner.
	RoleClusterChecks = "cluster-checks"

	// VariantA and VariantB are the variants of the two Agent installations, each running on
	// node pools of its own. The variants play symmetric roles, so that a benchmark can swap
	// the images of the installations half way through to cancel out the difference between
	// the machines of the node pools.
	VariantA = "a"
	VariantB = "b"

	// RoleTag is the host tag the node Agents derive from RoleLabel.
	RoleTag = "role"
	// VariantTag is the host tag the node Agents derive from VariantLabel.
	VariantTag = "variant"
	// AgentVariantTag is the tag of the KSM metrics set to the variant of the Agent
	// installation reporting them. VariantTag can't be used: the KSM metrics of a pod or a
	// node also carry the host tags of that node, unrelated to the reporting installation.
	AgentVariantTag = "agent_variant"
)

const nbNode = 3

// helmTimeoutSeconds is the timeout of the Helm releases of the Agents, longer than the
// 5-minute default, which an upgrade replacing the images of an installation can exceed.
const helmTimeoutSeconds = 15 * 60

// defaultInstanceType is the instance type of the node pools, unless
// ddinfra:aws/defaultInstanceType is set. The CPU usage of the Agents is compared on them,
// so it must not be burstable: CPU credits would make the results depend on the history of
// each node.
const defaultInstanceType = "m6i.large"

// nodePools are the node pools of each variant, by role, with their number of nodes.
var nodePools = []struct {
	role   string
	nbNode int
}{
	{role: RoleNodeAgent, nbNode: nbNode},
	{role: RoleClusterAgent, nbNode: 1},
	{role: RoleClusterChecks, nbNode: 1},
}

//go:embed simple_check.py
var simpleCheckPy string

// internalProfilingEnv makes the Agents upload their profiles. The chart only passes
// datadog.env[Dict] to the node Agent, so it is also set on the Cluster Agent and the
// cluster checks runner. It goes in envDict because Helm replaces lists from a later
// values file, which would drop the env entries of the e2e framework defaults.
var internalProfilingEnv = map[string]string{
	"DD_INTERNAL_PROFILING_BLOCK_PROFILE_RATE":           "10000",
	"DD_INTERNAL_PROFILING_ENABLE_BLOCK_PROFILING":       "true",
	"DD_INTERNAL_PROFILING_ENABLE_GOROUTINE_STACKTRACES": "true",
	"DD_INTERNAL_PROFILING_ENABLE_MUTEX_PROFILING":       "true",
	"DD_INTERNAL_PROFILING_ENABLED":                      "true",
	"DD_INTERNAL_PROFILING_MUTEX_PROFILE_FRACTION":       "100",
}

// apmSocketDir is the host directory of the trace Agent socket of the node Agent
// (datadog.apm.hostSocketPath and datadog.apm.socketPath chart defaults).
const apmSocketDir = "/var/run/datadog"

// Namespace returns the namespace of the Agent installation of variant.
func Namespace(variant string) string {
	return "datadog-" + variant
}

// Run is the entry point for the benchmarkeks scenario when run via Pulumi.
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	return RunWithEnv(ctx, awsEnv, outputs.NewBenchmarkEKS(), ParamsFromEnvironment(awsEnv))
}

// RunWithEnv deploys the benchmarkeks scenario using a provided env and params.
// It accepts the BenchmarkEKSOutputs interface, enabling reuse between provisioners and
// direct Pulumi runs.
func RunWithEnv(ctx *pulumi.Context, awsEnv resourcesAws.Environment, env outputs.BenchmarkEKSOutputs, params *RunParams) error {
	cluster, err := scenarioEks.NewCluster(awsEnv, "eks")
	if err != nil {
		return err
	}

	if err := cluster.Export(ctx, env.KubernetesClusterOutput()); err != nil {
		return err
	}

	instanceType := awsEnv.GetStringWithDefault(awsEnv.InfraConfig, resourcesAws.DDInfraDefaultInstanceTypeParamName, defaultInstanceType)
	for _, variant := range []string{VariantA, VariantB} {
		for _, pool := range nodePools {
			// The dependency on the cluster component makes the node groups wait for the
			// CNI custom-networking setup (ENIConfig + aws-node patch) before joining: a
			// dependsOn on a component resource extends to all of its children.
			if _, err := eks.NewManagedNodeGroup(ctx, "ng-"+pool.role+"-"+variant, &eks.ManagedNodeGroupArgs{
				Cluster:             cluster.Cluster.Core,
				InstanceTypes:       pulumi.ToStringArray([]string{instanceType}),
				ForceUpdateVersion:  pulumi.BoolPtr(true),
				NodeGroupNamePrefix: awsEnv.CommonNamer().DisplayName(37, pulumi.String("ng"), pulumi.String(pool.role), pulumi.String(variant)),
				ScalingConfig: awsEks.NodeGroupScalingConfigArgs{
					DesiredSize: pulumi.Int(pool.nbNode),
					MaxSize:     pulumi.Int(pool.nbNode),
					MinSize:     pulumi.Int(pool.nbNode),
				},
				NodeRole: cluster.Cluster.InstanceRoles.Index(pulumi.Int(0)),
				Labels: pulumi.StringMap{
					RoleLabel:    pulumi.String(pool.role),
					VariantLabel: pulumi.String(variant),
				},
				Taints: awsEks.NodeGroupTaintArray{taint(RoleLabel, pool.role), taint(VariantLabel, variant)},
				RemoteAccess: awsEks.NodeGroupRemoteAccessArgs{
					Ec2SshKey:              pulumi.StringPtr(awsEnv.DefaultKeyPairName()),
					SourceSecurityGroupIds: pulumi.ToStringArray(awsEnv.EKSAllowedInboundSecurityGroups()),
				},
			}, awsEnv.WithProviders(config.ProviderAWS, config.ProviderEKS), utils.PulumiDependsOn(cluster)); err != nil {
				return err
			}
		}
	}

	// The simulated nodes are part of the infrastructure: they don't depend on the Agent
	// images, so they are deployed in init-only mode too.
	if _, err := kwok.K8sAppDefinition(&awsEnv, cluster.KubeProvider); err != nil {
		return err
	}

	// The Agent installations and the churn orchestrator both require the Agent API
	// key (via Helm and the churn Fargate sidecar), which is only configured when the
	// Agent is deployed. Skip them when the Agent is disabled (--no-install-agent), and
	// in init-only mode, which provisions the infrastructure while the Agent images of
	// the pipeline are still being built.
	if awsEnv.InitOnly() || !awsEnv.AgentDeploy() {
		env.DisableAgents()
		return nil
	}

	var agentDeps []pulumi.Resource

	// The profiler uploads to a trace Agent on localhost:8126 by default, which only the
	// node Agent pod runs. The Cluster Agent and the cluster checks runner upload instead
	// to the trace Agent of the node Agent of their node, through its socket mounted from
	// the host.
	clusterProfilingEnv := maps.Clone(internalProfilingEnv)
	clusterProfilingEnv["DD_INTERNAL_PROFILING_UNIX_SOCKET"] = apmSocketDir + "/apm.socket"
	apmSocketVolumes := []map[string]any{
		{
			"name": "apmsocket",
			"hostPath": map[string]any{
				"path": apmSocketDir,
				"type": "DirectoryOrCreate",
			},
		},
	}
	apmSocketVolumeMounts := []map[string]any{
		{
			"name":      "apmsocket",
			"mountPath": apmSocketDir,
			"readOnly":  true,
		},
	}

	for _, param := range []struct {
		variant    string
		images     Images
		deployCRDs bool
		output     *agent.KubernetesAgentOutput
	}{
		{
			variant:    VariantA,
			images:     params.a,
			deployCRDs: true,
			output:     env.AgentAOutput(),
		},
		{
			variant:    VariantB,
			images:     params.b,
			deployCRDs: false,
			output:     env.AgentBOutput(),
		},
	} {
		kubernetesAgent, err := helm.NewKubernetesAgent(&awsEnv, awsEnv.Namer.ResourceName("datadog-agent", param.variant), cluster.KubeProvider,
			kubernetesagentparams.WithBaseName("dda-"+param.variant),
			kubernetesagentparams.WithNamespace(Namespace(param.variant)),
			kubernetesagentparams.WithClusterName(cluster.ClusterName),
			kubernetesagentparams.WithAgentFullImagePath(param.images.AgentFullImagePath),
			kubernetesagentparams.WithClusterAgentFullImagePath(param.images.ClusterAgentFullImagePath),
			kubernetesagentparams.WithAgentVersion(param.images.AgentVersion),
			kubernetesagentparams.WithClusterAgentVersion(param.images.ClusterAgentVersion),
			kubernetesagentparams.WithTimeout(helmTimeoutSeconds),
			kubernetesagentparams.WithHelmValues(utils.YAMLMustMarshal(map[string]any{
				"datadog": map[string]any{
					// Single Step Instrumentation requires the admission controller,
					// which is disabled below, and the chart refuses to render otherwise.
					"apm": map[string]any{
						"instrumentation": map[string]any{
							"enabled": false,
						},
					},
					"nodeLabelsAsTags": map[string]any{
						RoleLabel:    RoleTag,
						VariantLabel: VariantTag,
					},
					"podLabelsAsTags": map[string]any{
						"app": "app",
					},
					// Replaces the tags of the e2e framework defaults, which the benchmark doesn't use.
					"kubeStateMetricsCore": map[string]any{
						"tags": []string{AgentVariantTag + ":" + param.variant},
					},
					"envDict": internalProfilingEnv,
					"checksd": map[string]string{
						"simple_check.py": simpleCheckPy,
					},
					"operator": map[string]any{
						"enabled": false,
					},
				},
				"agents": map[string]any{
					// Replace all the node Agents of the variant at once on an upgrade, e.g.
					// when a benchmark swaps the images of the variants: one node at a time
					// (the 10% default) exceeds the timeout of the Helm release.
					"updateStrategy": map[string]any{
						"type": "RollingUpdate",
						"rollingUpdate": map[string]any{
							"maxUnavailable": "100%",
						},
					},
					"nodeSelector": map[string]any{
						VariantLabel: param.variant,
					},
					// The node Agent runs as a DaemonSet on every node of its variant
					// (all role pools), so it tolerates any role taint.
					"tolerations": []any{
						map[string]any{
							"key":      RoleLabel,
							"operator": "Exists",
							"effect":   "NoSchedule",
						},
						toleration(VariantLabel, param.variant),
					},
				},
				"clusterAgent": map[string]any{
					"replicas":     1,
					"envDict":      clusterProfilingEnv,
					"volumes":      apmSocketVolumes,
					"volumeMounts": apmSocketVolumeMounts,
					"nodeSelector": map[string]any{
						RoleLabel:    RoleClusterAgent,
						VariantLabel: param.variant,
					},
					"tolerations": []any{toleration(RoleLabel, RoleClusterAgent), toleration(VariantLabel, param.variant)},
					// The e2e framework defaults throttle the CPU and would OOM-kill the
					// Cluster Agent on a large simulated cluster. It has a node of its own, with
					// only the unmeasured node Agent of its variant, so generous limits starve
					// nothing measured and keep them out of the results.
					"resources": map[string]any{
						"requests": map[string]any{
							"cpu":    "200m",
							"memory": "256Mi",
						},
						"limits": map[string]any{
							"cpu":    "2",
							"memory": "2Gi",
						},
					},
					"metricsProvider": map[string]any{
						"registerAPIService": false,
					},
					"admissionController": map[string]any{
						// Disabled by default: both installs would otherwise reconcile the
						// same cluster-scoped datadog-webhook configurations and fight over
						// them. It can be re-enabled with --helm-config (applied to both
						// installs), preferably together with distinct webhookName values
						// and datadog.apm.instrumentation.enabled if needed.
						"enabled": false,
						"agentSidecarInjection": map[string]any{
							"clusterAgentCommunicationEnabled": false,
						},
					},
				},
				"clusterChecksRunner": map[string]any{
					"replicas":     1,
					"envDict":      clusterProfilingEnv,
					"volumes":      apmSocketVolumes,
					"volumeMounts": apmSocketVolumeMounts,
					"nodeSelector": map[string]any{
						RoleLabel:    RoleClusterChecks,
						VariantLabel: param.variant,
					},
					"tolerations": []any{toleration(RoleLabel, RoleClusterChecks), toleration(VariantLabel, param.variant)},
					// Same as for the Cluster Agent: the runner hosts the KSM check, whose
					// memory grows with the number of objects in the cluster. Its CPU limit
					// is above the node capacity, so it never throttles the bursts of KSM
					// check runs. The limit of the e2e framework defaults can't be removed
					// with a null instead: it reaches the manifest, read as a zero limit.
					// The memory limit leaves room for a leaking version to grow for a
					// while instead of being OOM-killed right away.
					"resources": map[string]any{
						"requests": map[string]any{
							"cpu":    "1",
							"memory": "512Mi",
						},
						"limits": map[string]any{
							"cpu":    "64",
							"memory": "5Gi",
						},
					},
					// The CPU requests of the runner and of the node Agent of its node leave
					// no room for a second runner, so the default surge of the chart can't
					// schedule the new runner next to the old one and a Helm upgrade of the
					// stack hangs. Replace the runner instead.
					"strategy": map[string]any{
						"rollingUpdate": map[string]any{
							"maxSurge":       0,
							"maxUnavailable": 1,
						},
					},
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
					},
				},
				// The DatadogInstrumentation CRD is rendered by a second alias of the
				// datadog-crds subchart (enabled by datadog.instrumentationCrd.enabled).
				// It must not also be enabled under datadog-crds above, or the release
				// renders it twice and fails with an "already exists" error.
				"datadog-instrumentation-crd": map[string]any{
					"crds": map[string]any{
						"datadogInstrumentations": param.deployCRDs,
					},
				},
			})),
		)
		if err != nil {
			return err
		}
		if err := kubernetesAgent.Export(ctx, param.output); err != nil {
			return err
		}
		agentDeps = append(agentDeps, kubernetesAgent)
	}

	// The churn pods are labeled for Fargate sidecar injection, so when the admission
	// controller is enabled they must wait for it to be ready, otherwise they would be
	// admitted without the Datadog sidecar. The label also schedules them on Fargate.
	if _, err := churn.K8sAppDefinition(&awsEnv, cluster.KubeProvider, utils.PulumiDependsOn(agentDeps...)); err != nil {
		return err
	}

	return nil
}

// taint returns the taint key=value of the nodes of a node pool, keeping off the pods that
// don't tolerate it.
func taint(key, value string) awsEks.NodeGroupTaintArgs {
	return awsEks.NodeGroupTaintArgs{
		Key:    pulumi.String(key),
		Value:  pulumi.String(value),
		Effect: pulumi.String("NO_SCHEDULE"),
	}
}

// toleration tolerates the taint key=value.
func toleration(key, value string) map[string]any {
	return map[string]any{
		"key":      key,
		"operator": "Equal",
		"value":    value,
		"effect":   "NoSchedule",
	}
}
