// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package oomkill deploys a workload that is repeatedly OOM-killed by its cgroup
// memory limit, to exercise the oom_kill eBPF check.
package oomkill

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

const (
	// Name is the name of the deployment and of its container.
	Name = "oomkill"
	// ProcessName is the name of the process that gets OOM-killed.
	ProcessName = "tail"
)

// K8sAppDefinition deploys, in the given namespace, a busybox loop (from the
// apps-stress-ng image, which is Alpine based) that runs `tail /dev/zero` under
// a 32Mi memory limit. tail buffers its input until it finds a newline, which
// never comes, so it gets OOM-killed by the kernel within milliseconds.
//
// With cgroup v1 or Kubernetes < 1.28, only tail is killed and the shell starts
// it again 10 seconds later. With cgroup v2 and Kubernetes >= 1.28, the kubelet
// sets memory.oom.group, so the whole container is killed: the pod goes into
// CrashLoopBackOff and gets OOM-killed again after every restart backoff (up to
// 5 minutes). The deployment is therefore never awaited by Pulumi.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", Name, k8sComponent, opts...); err != nil {
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

	var imagePullSecrets corev1.LocalObjectReferenceArray
	if e.ImagePullRegistry() != "" {
		imgPullSecret, err := utils.NewImagePullSecret(e, namespace, opts...)
		if err != nil {
			return nil, err
		}
		imagePullSecrets = append(imagePullSecrets, corev1.LocalObjectReferenceArgs{
			Name: imgPullSecret.Metadata.Name(),
		})
	}

	// Requests equal limits, so that the pod gets the Guaranteed QoS class.
	resources := pulumi.StringMap{
		"cpu":    pulumi.String("100m"),
		"memory": pulumi.String("32Mi"),
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), Name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(Name),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String(Name),
			},
			Annotations: pulumi.StringMap{
				// The pod may never become ready, see above.
				"pulumi.com/skipAwait": pulumi.String("true"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String(Name),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String(Name),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ImagePullSecrets: imagePullSecrets,
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String(Name),
							Image: pulumi.String(apps.Image(e, "apps-stress-ng")),
							// Override the stress-ng entrypoint with the busybox shell of the image.
							Command: pulumi.StringArray{
								pulumi.String("/bin/sh"),
								pulumi.String("-c"),
								pulumi.String("while true; do " + ProcessName + " /dev/zero; sleep 10; done"),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits:   resources,
								Requests: resources,
							},
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
