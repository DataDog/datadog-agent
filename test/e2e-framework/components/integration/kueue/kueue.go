// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package kueue installs the upstream Kueue control plane and wires it for the
// Datadog Agent OpenMetrics check.
//
// The component:
//   - applies the upstream Kueue release manifests (CRDs + RBAC + controller)
//     via server-side apply into the kueue-system namespace;
//   - binds the Datadog Agent node ServiceAccount to the kueue-metrics-reader
//     ClusterRole so the Agent can bearer-token scrape the HTTPS metrics endpoint
//     of kueue-controller-manager on :8443/metrics;
//   - provisions a ResourceFlavor / ClusterQueue / LocalQueue and a continuous
//     Job-producing workload so that Kueue admission metrics
//     (kueue_pending_workloads, kueue_admitted_active_workloads,
//     kueue_cluster_queue_status, kueue_admission_attempts_total, ...) are
//     populated for the check to observe.
package kueue

import (
	_ "embed"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	// Version is the upstream Kueue release used by this lab.
	Version = "v0.18.0"

	// Namespace is the namespace the upstream manifests install Kueue into.
	Namespace = "kueue-system"

	// MetricsReaderClusterRole is the ClusterRole shipped by Kueue that grants
	// access to the controller-manager metrics endpoint.
	MetricsReaderClusterRole = "kueue-metrics-reader"

	// queueName is the LocalQueue referenced by the sample workload.
	queueName = "lab-local-queue"
	// clusterQueueName is the ClusterQueue backing the LocalQueue.
	clusterQueueName = "lab-cluster-queue"
	// flavorName is the ResourceFlavor backing the ClusterQueue.
	flavorName = "lab-flavor"
)

// manifestURL returns the upstream Kueue release manifest URL for Version.
func manifestURL() string {
	return fmt.Sprintf("https://github.com/kubernetes-sigs/kueue/releases/download/%s/manifests.yaml", Version)
}

//go:embed manifests/queues.yaml
var queuesManifest string

//go:embed manifests/workload.yaml
var workloadManifest string

// K8sAppDefinition returns a workload-app function that installs Kueue and the
// supporting RBAC/queues/workload. agentNamespace and agentServiceAccount must
// match the Datadog Agent node-agent ServiceAccount so it can scrape the
// bearer-token protected metrics endpoint.
func K8sAppDefinition(agentNamespace, agentServiceAccount string) func(e config.Env, kubeProvider *kubernetes.Provider) (*componentskube.Workload, error) {
	return func(e config.Env, kubeProvider *kubernetes.Provider) (*componentskube.Workload, error) {
		k8sComponent := &componentskube.Workload{}
		if err := e.Ctx().RegisterComponentResource("dd:integration", "kueue", k8sComponent); err != nil {
			return nil, err
		}

		baseOpts := []pulumi.ResourceOption{
			pulumi.Provider(kubeProvider),
			pulumi.Parent(k8sComponent),
		}

		// 1. Install the upstream Kueue control plane (CRDs + RBAC + controller)
		// using server-side apply (the EKS KubeProvider has it enabled).
		controlPlane, err := yaml.NewConfigFile(e.Ctx(), "kueue-control-plane", &yaml.ConfigFileArgs{
			File: manifestURL(),
		}, baseOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to apply Kueue manifests: %w", err)
		}

		controlPlaneReady := append(baseOpts, utils.PulumiDependsOn(controlPlane))

		// 2. Bind the Datadog Agent node ServiceAccount to kueue-metrics-reader
		// so the Agent can bearer-token scrape the HTTPS metrics endpoint.
		if _, err := rbacv1.NewClusterRoleBinding(e.Ctx(), "kueue-metrics-reader-agent", &rbacv1.ClusterRoleBindingArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("datadog-agent-kueue-metrics-reader"),
			},
			RoleRef: &rbacv1.RoleRefArgs{
				ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
				Kind:     pulumi.String("ClusterRole"),
				Name:     pulumi.String(MetricsReaderClusterRole),
			},
			Subjects: rbacv1.SubjectArray{
				&rbacv1.SubjectArgs{
					Kind:      pulumi.String("ServiceAccount"),
					Name:      pulumi.String(agentServiceAccount),
					Namespace: pulumi.String(agentNamespace),
				},
			},
		}, controlPlaneReady...); err != nil {
			return nil, fmt.Errorf("failed to bind agent SA to %s: %w", MetricsReaderClusterRole, err)
		}

		// 3. Provision queues (ResourceFlavor + ClusterQueue + LocalQueue) once
		// the CRDs are established.
		queues, err := yaml.NewConfigGroup(e.Ctx(), "kueue-queues", &yaml.ConfigGroupArgs{
			YAML: []string{queuesManifest},
		}, controlPlaneReady...)
		if err != nil {
			return nil, fmt.Errorf("failed to apply Kueue queues: %w", err)
		}

		// 4. Deploy a continuous workload that keeps submitting Jobs to the
		// LocalQueue so admission/pending/active metrics stay populated.
		workloadNs, err := corev1.NewNamespace(e.Ctx(), "kueue-workload-ns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("kueue-workload"),
			},
		}, baseOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create workload namespace: %w", err)
		}

		workloadReady := append(baseOpts,
			utils.PulumiDependsOn(queues, workloadNs),
		)
		if _, err := yaml.NewConfigGroup(e.Ctx(), "kueue-workload", &yaml.ConfigGroupArgs{
			YAML: []string{workloadManifest},
		}, workloadReady...); err != nil {
			return nil, fmt.Errorf("failed to apply Kueue workload: %w", err)
		}

		return k8sComponent, nil
	}
}
