// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package jvmgc deploys a HotSpot JVM that continuously allocates memory, so
// that its garbage collector runs every few seconds. It exercises the jvm_memory
// eBPF check, which relies on the HotSpot USDT probes.
package jvmgc

import (
	_ "embed"
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

const (
	// Name is the name of the deployment and of its container.
	Name = "jvm-gc"

	// ImageName is the short name of the JVM image.
	ImageName = "eclipse-temurin"
	// ImageTag is the pinned tag of the JVM image. It must be a glibc-based
	// HotSpot build: only those ship the HotSpot USDT probes (`.note.stapsdt`
	// section of libjvm.so), the Alpine (musl) builds do not. It must also be a
	// JDK, not a JRE, because the source launcher needs the jdk.compiler module.
	ImageTag = "21.0.12.1_1-jdk-noble"
	// imageDigest pins the content of ImageTag (multi-arch index digest).
	imageDigest = "sha256:b468c3fc688b14450571494f588bd939378e7fd542ed5a73f8efc13f17872a87"

	// MaxHeapBytes is the maximum heap size of the JVM, passed with -Xmx.
	MaxHeapBytes = 128 * 1024 * 1024
)

//go:embed GcLoop.java
var gcLoopSource string

// K8sAppDefinition deploys, in the given namespace, a JVM running GcLoop.java
// with the G1 collector. The collector is pinned because, in a container with
// less than 2 CPUs or 1792 MiB of memory, the JVM ergonomics would select the
// Serial collector, whose memory pools have different names.
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

	cm, err := corev1.NewConfigMap(e.Ctx(), Name, &corev1.ConfigMapArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(Name),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String(Name),
			},
		},
		Data: pulumi.StringMap{
			"GcLoop.java": pulumi.String(gcLoopSource),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

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

	if _, err := appsv1.NewDeployment(e.Ctx(), Name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(Name),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String(Name),
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
							Image: pulumi.String(image(e)),
							// Bypass the entrypoint of the image (CA certificates setup) and
							// run java as PID 1.
							Command: pulumi.StringArray{
								pulumi.String("java"),
								pulumi.String("-XX:+UseG1GC"),
								pulumi.String("-Xms64m"),
								pulumi.String(fmt.Sprintf("-Xmx%dm", MaxHeapBytes>>20)),
								// One line per collection in the container logs, to debug the test.
								pulumi.String("-Xlog:gc"),
								pulumi.String("/app/GcLoop.java"),
							},
							// The JVM uses about 125 MiB at peak and a few percent of a CPU.
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("500m"),
									"memory": pulumi.String("256Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("256Mi"),
								},
							},
							VolumeMounts: corev1.VolumeMountArray{
								corev1.VolumeMountArgs{
									Name:      pulumi.String("app"),
									MountPath: pulumi.String("/app"),
									ReadOnly:  pulumi.Bool(true),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						corev1.VolumeArgs{
							Name: pulumi.String("app"),
							ConfigMap: &corev1.ConfigMapVolumeSourceArgs{
								Name: cm.Metadata.Name(),
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

// image returns the reference of the JVM image, a Docker Hub official image.
// On AWS, the internal Docker Hub mirror is the ECR pull-through cache, which
// requires the credentials of the image pull secret of the namespace: that
// secret only exists when an image pull registry is configured. Without one
// (some local runs), the image is pulled from Docker Hub.
func image(e config.Env) string {
	ref := "library/" + ImageName + ":" + ImageTag + "@" + imageDigest
	if e.ImagePullRegistry() != "" {
		return e.InternalDockerhubMirror() + "/" + ref
	}
	return ref
}
