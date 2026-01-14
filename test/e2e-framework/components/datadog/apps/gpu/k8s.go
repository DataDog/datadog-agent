// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package gpu provides a GPU workload component for testing GPU monitoring.
package gpu

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// K8sAppDefinition deploys a GPU workload (cuda-basic) as a DaemonSet to all GPU nodes.
// The workload runs a simple CUDA vector addition program that requires a GPU.
// This is useful for testing GPU monitoring and container-to-GPU mapping.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "gpu", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	// Deploy cuda-basic workload as DaemonSet to run on all GPU nodes
	// Each pod runs a simple CUDA vector addition in a loop for testing
	if _, err := appsv1.NewDaemonSet(e.Ctx(), "cuda-basic", &appsv1.DaemonSetArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("cuda-basic"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("cuda-basic"),
			},
		},
		Spec: &appsv1.DaemonSetSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("cuda-basic"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("cuda-basic"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String("cuda-basic"),
							Image: pulumi.String("ghcr.io/datadog/apps-cuda-basic:" + apps.Version),
							// Run cuda-basic in a loop: 50000 elements, 1000 loops, 0s wait
							// Then sleep 5s between iterations for continuous GPU activity
							Command: pulumi.StringArray{
								pulumi.String("/bin/sh"),
								pulumi.String("-c"),
								pulumi.String("while true; do /usr/local/bin/cuda-basic 50000 1000 0; sleep 5; done"),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									// Request 1 GPU - this triggers NVIDIA device plugin
									// to set NVIDIA_VISIBLE_DEVICES in the container spec
									"nvidia.com/gpu": pulumi.String("1"),
								},
							},
						},
					},
					// Tolerate GPU node taints (if any)
					Tolerations: corev1.TolerationArray{
						corev1.TolerationArgs{
							Key:      pulumi.String("nvidia.com/gpu"),
							Operator: pulumi.String("Exists"),
							Effect:   pulumi.String("NoSchedule"),
						},
					},
					// Only schedule on GPU nodes (labeled by e2e-framework)
					NodeSelector: pulumi.StringMap{
						"accelerator": pulumi.String("nvidia-gpu"),
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
