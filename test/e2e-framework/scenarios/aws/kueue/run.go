// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kueue is the Pulumi scenario for the Kueue integration lab.
//
// It provisions a single-node EKS cluster (reusing the aws/eks cluster builder),
// installs the upstream Kueue control plane plus a continuous workload via the
// kueue integration component, and deploys the Datadog Agent through Helm with an
// OpenMetrics check configured to scrape the Kueue controller-manager metrics
// endpoint (HTTPS :8443/metrics, tls_verify:false, ServiceAccount-token auth).
package kueue

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/helm"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/kubernetesagentparams"
	integrationkueue "github.com/DataDog/datadog-agent/test/e2e-framework/components/integration/kueue"
	resourcesAws "github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/eks"
	"github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/outputs"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// clusterName matches plan.cluster_name in lab.json and the cluster output
	// key dd-Cluster-eks consumed by the invoke task module.
	clusterName = "eks"

	// agentNamespace is the namespace the Datadog Agent is deployed into.
	agentNamespace = "datadog"
	// agentServiceAccount is the node-agent ServiceAccount created by the
	// Datadog Helm chart; it is bound to kueue-metrics-reader by the component.
	// helm.NewKubernetesAgent installs the chart as release "dda-<platform>", so
	// the Linux node-agent SA (and DaemonSet) is named "dda-linux-datadog".
	agentServiceAccount = "dda-linux-datadog"
)

// confdHelmValues renders the OpenMetrics Kueue check into the Agent via the
// Datadog Helm chart datadog.confd map. The file name openmetrics.yaml makes the
// running check name `openmetrics`.
//
// Authentication note: the OpenMetrics v2 check (openmetrics_endpoint) only
// auto-attaches the projected ServiceAccount token (bearer_token_auth +
// bearer_token_path) when the scraped host is the Kubernetes API server. For an
// arbitrary controller-runtime-secured endpoint like Kueue's metrics service
// that bearer_token_path silently sends no Authorization header, yielding HTTP
// 401 (authentication failure, not a 403 RBAC denial). The documented way to
// send the SA token to a non-apiserver endpoint is the explicit auth_token
// reader/writer block: read the mounted token file and write it into the
// Authorization header. The literal "<TOKEN>" placeholder is intentional — the
// Agent substitutes the file contents at scrape time. The pattern strips the
// trailing newline from the token file.
//
// Metric coverage note: OpenMetrics v2 interprets every 'metrics' list entry as
// a regular expression, so the single entry 'kueue_.*' collects the full kueue_*
// family the controller-manager exposes. A previous fixed ~15-name allowlist
// undercollected — only 11 of the 27 exposed kueue_* families were captured, and
// 4 of the allowlisted names (kueue_evicted_workloads_total,
// kueue_cluster_queue_nominal_quota, kueue_cluster_queue_borrowing_limit,
// kueue_cluster_queue_resource_usage) did not exist on this Kueue version's
// endpoint at all. The wildcard keeps the lab a complete showcase of the
// integration and is resilient to Kueue renaming/adding metrics across versions.
const confdHelmValues = `
datadog:
  confd:
    openmetrics.yaml: |-
      init_config:
      instances:
        - openmetrics_endpoint: https://kueue-controller-manager-metrics-service.kueue-system.svc:8443/metrics
          namespace: kueue
          tls_verify: false
          auth_token:
            reader:
              type: file
              path: /var/run/secrets/kubernetes.io/serviceaccount/token
              pattern: (.+)
            writer:
              type: header
              name: Authorization
              value: "Bearer <TOKEN>"
          min_collection_interval: 30
          metrics:
            - kueue_.*
`

// Run is the Pulumi entry point registered as scenario "aws/kueue".
func Run(ctx *pulumi.Context) error {
	awsEnv, err := resourcesAws.NewEnvironment(ctx)
	if err != nil {
		return err
	}

	env := outputs.NewKubernetes()

	// Single Linux node group is sufficient (capacity_plan: 1x t3.medium AL2023).
	cluster, err := eks.NewCluster(awsEnv, clusterName,
		eks.WithLinuxNodeGroup(),
		eks.WithoutFargate(),
	)
	if err != nil {
		return err
	}

	if err := cluster.Export(ctx, env.KubernetesClusterOutput()); err != nil {
		return err
	}

	if awsEnv.InitOnly() {
		return nil
	}

	// Deploy the Datadog Agent via Helm with the OpenMetrics Kueue check.
	kubernetesAgent, err := helm.NewKubernetesAgent(&awsEnv, clusterName, cluster.KubeProvider,
		kubernetesagentparams.WithNamespace(agentNamespace),
		kubernetesagentparams.WithHelmValues(confdHelmValues),
		kubernetesagentparams.WithPulumiResourceOptions(utils.PulumiDependsOn(cluster)),
		kubernetesagentparams.WithTags([]string{"stackid:" + ctx.Stack()}),
	)
	if err != nil {
		return err
	}
	if err := kubernetesAgent.Export(ctx, env.KubernetesAgentOutput()); err != nil {
		return err
	}

	// Install Kueue control plane + RBAC + queues + continuous workload.
	if _, err := integrationkueue.K8sAppDefinition(agentNamespace, agentServiceAccount)(&awsEnv, cluster.KubeProvider); err != nil {
		return err
	}

	// This scenario does not use fakeintake; validation is via the live Agent.
	env.DisableFakeIntake()

	return nil
}
