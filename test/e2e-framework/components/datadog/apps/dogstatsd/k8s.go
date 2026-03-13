// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package dogstatsd

import (
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	rbacv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/rbac/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, statsdPort int, statsdSocket string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("dogstatsd-%d", statsdPort), k8sComponent, opts...); err != nil {
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

	// openshift requires a non-default service account tighted to the privileged scc
	sa, err := corev1.NewServiceAccount(e.Ctx(), fmt.Sprintf("dogstatsd-%d", statsdPort), &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("dogstatsd-sa"),
			Namespace: pulumi.StringPtr(namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// create a clusterRoleBinding to bind the new service account with the existing privileged scc
	if _, err := rbacv1.NewRoleBinding(e.Ctx(), fmt.Sprintf("dogstatsd-uds-scc-binding-%d", statsdPort), &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-scc-binding"),
			Namespace: pulumi.String(namespace),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:openshift:scc:privileged"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      sa.Metadata.Name().Elem(),
				Namespace: pulumi.String(namespace),
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-uds-with-csi-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-uds-with-csi"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-uds-with-csi"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-uds-with-csi"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"admission.datadoghq.com/config.mode": pulumi.String("csi"),
						"app":                                 pulumi.String("dogstatsd-uds-with-csi"),
						"admission.datadoghq.com/enabled":     pulumi.String("true"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),

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

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-uds-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-uds"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-uds"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-uds"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("dogstatsd-uds"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.String("unix:///var/dsd.socket"),
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
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("dogstatsd-socket"),
									MountPath: pulumi.String("/var/dsd.socket"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("dogstatsd-socket"),
							HostPath: &corev1.HostPathVolumeSourceArgs{
								Path: pulumi.String(statsdSocket),
								Type: pulumi.StringPtr("Socket"),
							},
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-udp-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-udp"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-udp"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-udp"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("dogstatsd-udp"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name: pulumi.String("HOST_IP"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("status.hostIP"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.Sprintf("$(HOST_IP):%d", statsdPort),
								},
								&corev1.EnvVarArgs{
									Name: pulumi.String("DD_ENTITY_ID"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("metadata.uid"),
										},
									},
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("2m"),
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

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-udp-origin-detection-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-udp-origin-detection"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-udp-origin-detection"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-udp-origin-detection"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("dogstatsd-udp-origin-detection"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name: pulumi.String("HOST_IP"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("status.hostIP"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.Sprintf("$(HOST_IP):%d", statsdPort),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("2m"),
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

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-udp-contname-injected-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-udp-contname-injected"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-udp-contname-injected"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-udp-contname-injected"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("dogstatsd-udp-contname-injected"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name: pulumi.String("HOST_IP"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("status.hostIP"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.Sprintf("$(HOST_IP):%d", statsdPort),
								},
								&corev1.EnvVarArgs{
									Name: pulumi.String("DD_INTERNAL_POD_UID"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("metadata.uid"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("DD_ENTITY_ID"),
									Value: pulumi.String("en-$(DD_INTERNAL_POD_UID)/dogstatsd"),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("2m"),
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

	if _, err := appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("dogstatsd-udp-external-data-only-%d", statsdPort), &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("dogstatsd-udp-external-data-only"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("dogstatsd-udp-external-data-only"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("dogstatsd-udp-external-data-only"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":                             pulumi.String("dogstatsd-udp-external-data-only"),
						"admission.datadoghq.com/enabled": pulumi.String("true"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("dogstatsd"),
							Image: pulumi.String("ghcr.io/datadog/apps-dogstatsd:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name: pulumi.String("HOST_IP"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("status.hostIP"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("STATSD_URL"),
									Value: pulumi.Sprintf("$(HOST_IP):%d", statsdPort),
								},
								&corev1.EnvVarArgs{
									Name: pulumi.String("DD_ENTITY_ID"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("metadata.uid"),
										},
									},
								},
								&corev1.EnvVarArgs{
									Name:  pulumi.String("DD_EXTERNAL_DATA_ONLY"),
									Value: pulumi.String("true"),
								},
							},
							Resources: &corev1.ResourceRequirementsArgs{
								Limits: pulumi.StringMap{
									"cpu":    pulumi.String("10m"),
									"memory": pulumi.String("32Mi"),
								},
								Requests: pulumi.StringMap{
									"cpu":    pulumi.String("2m"),
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
