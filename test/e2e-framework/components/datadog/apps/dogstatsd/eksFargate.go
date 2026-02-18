// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dogstatsd

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	sidecar "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/k8ssidecar"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func EksFargateAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, clusterAgentToken pulumi.StringInput, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	eksFargateComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "dogstatsd-fargate", eksFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(eksFargateComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	serviceAccount, err := sidecar.NewServiceAccountWithClusterPermissions(e.Ctx(), namespace, e.AgentAPIKey(), clusterAgentToken, opts...)

	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), "dogstatsd-fargate", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-fargate"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-fargate"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-fargate"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":                             pulumi.String("dogstatsd-fargate"),
						"agent.datadoghq.com/sidecar":     pulumi.String("fargate"),
						"admission.datadoghq.com/enabled": pulumi.String("true"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: serviceAccount.Metadata.Name(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.String("$(DD_DOGSTATSD_URL)"),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
							},
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return eksFargateComponent, nil
}
