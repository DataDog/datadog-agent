// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package churn deploys the "churn" orchestrator, which continuously creates and
// deletes Kubernetes workloads (fake nodes, sleeper DaemonSets, fake Deployments and
// Services) to exercise the Agent under a realistic, dynamic object population.
package churn

import (
	_ "embed"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/k8ssidecar"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

var (
	//go:embed workload/kwok-nodes.yaml
	workloadKwokNodes string

	//go:embed workload/sleepers.yaml
	workloadSleepers string

	//go:embed workload/fake.yaml
	workloadFake string
)

// K8sAppDefinition deploys the churn orchestrator and the workload manifests it
// drives.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "churn", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), "churn", &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.StringPtr("churn"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// Everything below lives in the "churn" namespace, so make it a dependency to
	// avoid a namespace-not-found race during apply.
	opts = append(opts, utils.PulumiDependsOn(ns))

	sa, err := k8ssidecar.NewServiceAccountWithClusterPermissions(e.Ctx(), "churn", e.AgentAPIKey(), pulumi.String(""), opts...)
	if err != nil {
		return nil, err
	}

	if _, err := rbacv1.NewClusterRoleBinding(e.Ctx(), "churn", &rbacv1.ClusterRoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.StringPtr("churn"),
		},
		RoleRef: rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("cluster-admin"),
		},
		Subjects: rbacv1.SubjectArray{
			&rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      sa.Metadata.Name().Elem(),
				Namespace: ns.Metadata.Name(),
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	cfgMap, err := corev1.NewConfigMap(e.Ctx(), "workload", &corev1.ConfigMapArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("workload"),
			Namespace: ns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app": pulumi.String("churn"),
			},
		},
		Data: pulumi.StringMap{
			"kwok-nodes.yaml": pulumi.String(workloadKwokNodes),
			"sleepers.yaml":   pulumi.String(workloadSleepers),
			"fake.yaml":       pulumi.String(workloadFake),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), "churn", &appsv1.DeploymentArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("churn"),
			Namespace: ns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"app": pulumi.String("churn"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("churn"),
				},
			},
			Replicas: pulumi.IntPtr(1),
			Template: corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":                         pulumi.String("churn"),
						"agent.datadoghq.com/sidecar": pulumi.String("fargate"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("churn"),
							Image: pulumi.String("ghcr.io/datadog/apps-churn:" + apps.Version),
							Args: pulumi.StringArray{
								pulumi.String("--manifests-dir"),
								pulumi.String("/etc/churn/..data/"),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("500m"),
									"memory": pulumi.String("1Gi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("500m"),
									"memory": pulumi.String("1Gi"),
								},
							},
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("workload"),
									MountPath: pulumi.String("/etc/churn"),
									ReadOnly:  pulumi.BoolPtr(true),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("workload"),
							ConfigMap: corev1.ConfigMapVolumeSourceArgs{
								Name: cfgMap.Metadata.Name(),
							},
						},
					},
					ServiceAccountName:            sa.Metadata.Name(),
					TerminationGracePeriodSeconds: pulumi.IntPtr(300),
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
