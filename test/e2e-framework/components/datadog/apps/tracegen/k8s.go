// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package tracegen

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

func generateTracegenTCPSpec(namespace string) *appsv1.DeploymentArgs {
	return &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("tracegen-tcp"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("tracegen-tcp"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("tracegen-tcp"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("tracegen-tcp"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("tracegen-tcp"),
							Image: pulumi.String("ghcr.io/datadog/apps-tracegen:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name: pulumi.String("DD_AGENT_HOST"),
									ValueFrom: &corev1.EnvVarSourceArgs{
										FieldRef: &corev1.ObjectFieldSelectorArgs{
											FieldPath: pulumi.String("status.hostIP"),
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
	}
}

func generateTracegenUdsSpec(namespace string, serviceAccountName pulumi.StringPtrInput) *appsv1.DeploymentArgs {
	return &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("tracegen-uds"),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String("tracegen-uds"),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String("tracegen-uds"),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String("tracegen-uds"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: serviceAccountName,
					Containers: corev1.ContainerArray{
						&corev1.ContainerArgs{
							Name:  pulumi.String("tracegen-uds"),
							Image: pulumi.String("ghcr.io/datadog/apps-tracegen:" + apps.Version),
							Env: &corev1.EnvVarArray{
								&corev1.EnvVarArgs{
									Name:  pulumi.String("DD_TRACE_AGENT_URL"),
									Value: pulumi.String("unix:///var/run/datadog/apm.socket"),
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
							VolumeMounts: &corev1.VolumeMountArray{
								&corev1.VolumeMountArgs{
									Name:      pulumi.String("apmsocketpath"),
									MountPath: pulumi.String("/var/run/datadog"),
								},
							},
						},
					},
					Volumes: corev1.VolumeArray{
						&corev1.VolumeArgs{
							Name: pulumi.String("apmsocketpath"),
							HostPath: &corev1.HostPathVolumeSourceArgs{
								Path: pulumi.String("/var/run/datadog"),
							},
						},
					},
				},
			},
		},
	}
}

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", fmt.Sprintf("%s-tracegen", namespace), k8sComponent, opts...); err != nil {
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
	sa, err := corev1.NewServiceAccount(e.Ctx(), e.CommonNamer().ResourceName("tracegen-uds-sa"), &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("tracegen-uds-sa"),
			Namespace: pulumi.StringPtr("workload-tracegen"),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	// create a clusterRoleBinding to bind the new service account with the existing privileged scc
	if _, err := rbacv1.NewRoleBinding(e.Ctx(), e.CommonNamer().ResourceName("tracegen-uds-scc-binding"), &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("tracegen-scc-binding"),
			Namespace: pulumi.String("workload-tracegen"),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:openshift:scc:hostaccess"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      sa.Metadata.Name().Elem(),
				Namespace: pulumi.String("workload-tracegen"),
			},
		},
	}, opts...); err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/tracegen-uds", generateTracegenUdsSpec(namespace, sa.Metadata.Name().Elem()), opts...); err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/tracegen-tcp", generateTracegenTCPSpec(namespace), opts...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}
