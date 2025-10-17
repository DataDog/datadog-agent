package dogstatsd

import (
	"fmt"

	"github.com/DataDog/test-infra-definitions/common/config"
	"github.com/DataDog/test-infra-definitions/common/utils"
	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	componentskube "github.com/DataDog/test-infra-definitions/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// K8sAppDefinitionOpenShift creates only UDP deployments for OpenShift
func K8sAppDefinitionOpenShift(e config.Env, kubeProvider *kubernetes.Provider, namespace string, statsdPort int, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("dogstatsd-openshift-%d", statsdPort), k8sComponent, opts...); err != nil {
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
						"app": pulumi.String("dogstatsd-udp-external-data-only"),
					},
				},
				Spec: &corev1.PodSpecArgs{
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
