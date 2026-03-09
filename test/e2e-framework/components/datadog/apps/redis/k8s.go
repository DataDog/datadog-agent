// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package redis

import (
	"github.com/Masterminds/semver/v3"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	autoscalingv2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2"
	autoscalingv2beta2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/autoscaling/v2beta2"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	policyv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1"
	policyv1beta1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/policy/v1beta1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, withDatadogAutoscaling bool, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "redis", k8sComponent, opts...); err != nil {
		return nil, err
	}

	kubeVersion, err := semver.NewVersion(utils.ParseKubernetesVersion(e.KubernetesVersion()))
	if err != nil {
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

	if _, err := appsv1.NewDeployment(e.Ctx(), "redis", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("redis"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app":  pulumi.String("redis"),
				"team": pulumi.String("container-integrations"), // Test auto_team_tag_collection feature (default enabled)
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("redis"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("redis"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("redis"),
							Image: pulumi.String("ghcr.io/datadog/redis:" + apps.Version),
							Args: pulumi.StringArray{
								pulumi.String("--loglevel"),
								pulumi.String("verbose"),
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
							Ports: &corev1.ContainerPortArray{
								&corev1.ContainerPortArgs{
									Name:          pulumi.String("redis"),
									ContainerPort: pulumi.Int(6379),
									Protocol:      pulumi.String("TCP"),
								},
							},
							LivenessProbe: &corev1.ProbeArgs{
								TcpSocket: &corev1.TCPSocketActionArgs{
									Port: pulumi.Int(6379),
								},
							},
							ReadinessProbe: &corev1.ProbeArgs{
								TcpSocket: &corev1.TCPSocketActionArgs{
									Port: pulumi.Int(6379),
								},
							},
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("redis-data"),
									MountPath: pulumi.String("/data"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name:     pulumi.String("redis-data"),
							EmptyDir: &corev1.EmptyDirVolumeSourceArgs{},
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	// In versions older than 1.21.0, we should use policyv1beta1
	kubeThresholdVersion, _ := semver.NewVersion("1.21.0")

	if kubeVersion.Compare(kubeThresholdVersion) < 0 {
		if _, err := policyv1beta1.NewPodDisruptionBudget(e.Ctx(), "redis", &policyv1beta1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("redis"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("redis"),
				},
			},
			Spec: &policyv1beta1.PodDisruptionBudgetSpecArgs{
				MaxUnavailable: pulumi.Int(1),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app": pulumi.String("redis"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	} else {
		if _, err := policyv1.NewPodDisruptionBudget(e.Ctx(), "redis", &policyv1.PodDisruptionBudgetArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("redis"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("redis"),
				},
			},
			Spec: &policyv1.PodDisruptionBudgetSpecArgs{
				MaxUnavailable: pulumi.Int(1),
				Selector: &metav1.LabelSelectorArgs{
					MatchLabels: pulumi.StringMap{
						"app": pulumi.String("redis"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	}

	if withDatadogAutoscaling && e.AgentDeploy() {
		ddm, err := apiextensions.NewCustomResource(e.Ctx(), "redis", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("datadoghq.com/v1alpha1"),
			Kind:       pulumi.String("DatadogMetric"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("redis"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("redis"),
				},
			},
			OtherFields: map[string]interface{}{
				"spec": pulumi.Map{
					"query": pulumi.Sprintf("avg:redis.net.instantaneous_ops_per_sec{kube_cluster_name:%%%%tag_kube_cluster_name%%%%,kube_namespace:%s,kube_deployment:redis}.rollup(60)", namespace),
				},
			},
		}, opts...)
		if err != nil {
			return nil, err
		}

		// In versions older than 1.23.0, we should use autoscalingv2beta2
		kubeThresholdVersion, _ = semver.NewVersion("1.23.0")

		if kubeVersion.Compare(kubeThresholdVersion) < 0 {
			if _, err := autoscalingv2beta2.NewHorizontalPodAutoscaler(e.Ctx(), "redis", &autoscalingv2beta2.HorizontalPodAutoscalerArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("redis"),
					Namespace: pulumi.String(namespace),
					Labels: pulumi.StringMap{
						"app": pulumi.String("redis"),
					},
				},
				Spec: &autoscalingv2beta2.HorizontalPodAutoscalerSpecArgs{
					MinReplicas: pulumi.Int(1),
					MaxReplicas: pulumi.Int(5),
					ScaleTargetRef: &autoscalingv2beta2.CrossVersionObjectReferenceArgs{
						ApiVersion: pulumi.String("apps/v1"),
						Kind:       pulumi.String("Deployment"),
						Name:       pulumi.String("redis"),
					},
					Metrics: &autoscalingv2beta2.MetricSpecArray{
						&autoscalingv2beta2.MetricSpecArgs{
							Type: pulumi.String("External"),
							External: &autoscalingv2beta2.ExternalMetricSourceArgs{
								Metric: &autoscalingv2beta2.MetricIdentifierArgs{
									Name: pulumi.String("datadogmetric@" + namespace + ":redis"),
								},
								Target: &autoscalingv2beta2.MetricTargetArgs{
									Type:         pulumi.String("AverageValue"),
									AverageValue: pulumi.String("10"),
								},
							},
						},
					},
				},
			}, append(opts, utils.PulumiDependsOn(ddm))...); err != nil {
				return nil, err
			}
		} else {
			if _, err := autoscalingv2.NewHorizontalPodAutoscaler(e.Ctx(), "redis", &autoscalingv2.HorizontalPodAutoscalerArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("redis"),
					Namespace: pulumi.String(namespace),
					Labels: pulumi.StringMap{
						"app": pulumi.String("redis"),
					},
				},
				Spec: &autoscalingv2.HorizontalPodAutoscalerSpecArgs{
					MinReplicas: pulumi.Int(1),
					MaxReplicas: pulumi.Int(5),
					ScaleTargetRef: &autoscalingv2.CrossVersionObjectReferenceArgs{
						ApiVersion: pulumi.String("apps/v1"),
						Kind:       pulumi.String("Deployment"),
						Name:       pulumi.String("redis"),
					},
					Metrics: &autoscalingv2.MetricSpecArray{
						&autoscalingv2.MetricSpecArgs{
							Type: pulumi.String("External"),
							External: &autoscalingv2.ExternalMetricSourceArgs{
								Metric: &autoscalingv2.MetricIdentifierArgs{
									Name: pulumi.String("datadogmetric@" + namespace + ":redis"),
								},
								Target: &autoscalingv2.MetricTargetArgs{
									Type:         pulumi.String("AverageValue"),
									AverageValue: pulumi.String("10"),
								},
							},
						},
					},
				},
			}, append(opts, utils.PulumiDependsOn(ddm))...); err != nil {
				return nil, err
			}
		}

		if _, err := apiextensions.NewCustomResource(e.Ctx(), "redis", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("autoscaling.k8s.io/v1"),
			Kind:       pulumi.String("VerticalPodAutoscaler"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("redis"),
				Namespace: pulumi.String(namespace),
				Labels: pulumi.StringMap{
					"app": pulumi.String("redis"),
				},
			},
			OtherFields: map[string]interface{}{
				"spec": pulumi.Map{
					"targetRef": pulumi.Map{
						"apiVersion": pulumi.String("apps/v1"),
						"kind":       pulumi.String("Deployment"),
						"name":       pulumi.String("redis"),
					},
					"updatePolicy": pulumi.Map{
						"updateMode": pulumi.String("Auto"),
					},
				},
			},
		}, opts...); err != nil {
			return nil, err
		}
	}

	if _, err := corev1.NewService(e.Ctx(), "redis", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("redis"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("redis"),
			},
		},
		Spec: &corev1.ServiceSpecArgs{
			Selector: pulumi.StringMap{
				"app": pulumi.String("redis"),
			},
			Ports: &corev1.ServicePortArray{
				&corev1.ServicePortArgs{
					Name:       pulumi.String("redis"),
					Port:       pulumi.Int(6379),
					TargetPort: pulumi.String("redis"),
					Protocol:   pulumi.String("TCP"),
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), "redis-query", &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("redis-query"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("redis-query"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("redis-query"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("redis-query"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("query"),
							Image: pulumi.String("ghcr.io/datadog/apps-redis-client:" + apps.Version),
							Args: pulumi.StringArray{
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

	return k8sComponent, nil
}
