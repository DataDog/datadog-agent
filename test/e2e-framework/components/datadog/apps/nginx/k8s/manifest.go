// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package k8s

import (
	"strconv"

	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
)

// runtimeClassToPulumi converts a runtime class name to a pulumi.StringInput.
// If runtimeClass is empty, it returns nil.
func runtimeClassToPulumi(runtimeClass string) pulumi.StringInput {
	if runtimeClass == "" {
		return nil
	}
	return pulumi.String(runtimeClass)
}

// NewNginxDeploymentManifest creates a new deployment manifest for Nginx server
func NewNginxDeploymentManifest(namespace string, nginxPort int, mods ...DeploymentModifier) (*appsv1.DeploymentArgs, error) {
	manifest := &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("nginx"),
			Namespace: pulumi.String(namespace),
			Annotations: pulumi.StringMap{
				"x-sub-team": pulumi.String("contint"),
			},
			Labels: pulumi.StringMap{
				"app":    pulumi.String("nginx"),
				"x-team": pulumi.String("contp"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("nginx"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":           pulumi.String("nginx"), // required for service
						"x-parent-type": pulumi.String("deployment"),
					},
					Annotations: pulumi.StringMap{
						"x-parent-name": pulumi.String("nginx"),
						"ad.datadoghq.com/nginx.checks": pulumi.String(utils.JSONMustMarshal(
							map[string]interface{}{
								"nginx": map[string]interface{}{
									"init_config":           map[string]interface{}{},
									"check_tag_cardinality": "high",
									"instances": []map[string]interface{}{
										{
											"nginx_status_url": "http://%%host%%:" + pulumi.String(strconv.Itoa(nginxPort)) + "/nginx_status",
										},
									},
								},
							},
						)),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("nginx"),
							Image: pulumi.String("ghcr.io/datadog/apps-nginx-server:" + apps.Version),
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
							Ports: &corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("http"),
									ContainerPort: pulumi.Int(nginxPort),
									Protocol:      pulumi.String("TCP"),
								},
							},
							LivenessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Port: pulumi.Int(nginxPort),
								},
								TimeoutSeconds: pulumi.Int(5),
							},
							ReadinessProbe: &corev1.ProbeArgs{
								HttpGet: &corev1.HTTPGetActionArgs{
									Port: pulumi.Int(nginxPort),
								},
								TimeoutSeconds: pulumi.Int(5),
							},
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("cache"),
									MountPath: pulumi.String("/var/cache/nginx"),
								},
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("var-run"),
									MountPath: pulumi.String("/var/run"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name:     pulumi.String("cache"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
						},
						&corev1.VolumeArgs{
							Name:     pulumi.String("var-run"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
						},
					},
				},
			},
		},
	}

	for _, mod := range mods {
		err := mod(manifest)
		if err != nil {
			return nil, err
		}
	}

	return manifest, nil
}

// NewNginxQueryDeploymentManifest creates a new deployment manifest for Nginx query app
func NewNginxQueryDeploymentManifest(namespace string, mods ...DeploymentModifier) (*appsv1.DeploymentArgs, error) {
	manifest := &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("nginx-query"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("nginx-query"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("nginx-query"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("nginx-query"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("query"),
							Image: pulumi.String("ghcr.io/datadog/apps-http-client:" + apps.Version),
							Args: pulumi.StringArray{
								pulumi.String("-url"),
								pulumi.String("http://nginx"),
								pulumi.String("-min-tps"),
								pulumi.String("1"),
								pulumi.String("-max-tps"),
								pulumi.String("60"),
								pulumi.String("-period"),
								pulumi.String("20m"),
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("100m"),
									"memory": pulumi.String("64Mi"),
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
	}

	for _, mod := range mods {
		err := mod(manifest)
		if err != nil {
			return nil, err
		}
	}

	return manifest, nil
}

// NewNginxServiceManifest creates a new service manifest for the Nginx deployment
func NewNginxServiceManifest(namespace string, nginxPort int) *corev1.ServiceArgs {
	return &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("nginx"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("nginx"),
			},
			Annotations: pulumi.StringMap{
				"ad.datadoghq.com/service.checks": pulumi.String(utils.JSONMustMarshal(
					map[string]interface{}{
						"http_check": map[string]interface{}{
							"init_config": map[string]interface{}{},
							"instances": []map[string]interface{}{
								{
									"name":    "My Nginx",
									"url":     "http://%%host%%",
									"timeout": 1,
								},
							},
						},
					},
				)),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{
				"app": pulumi.String("nginx"), // deployment is hardcoded above
			},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Port:       pulumi.Int(80),
					TargetPort: pulumi.String("http"),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}
}
