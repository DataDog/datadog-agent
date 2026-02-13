// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mutatedbyadmissioncontroller

import (
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

// K8sAppDefinition creates 3 different deployments:
// - to be mutated but without lib injection
// - to be mutated with lib injection specifying the library in an annotation.
// - to be mutated with lib injection without specifying the library in an
// annotation. The language will be auto-detected.
//
// Lib injection can be enabled or disabled by namespace. We use 2 separate
// namespaces so that we can test mutation with and without lib injection
// separately.
func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespaceWithoutLibInjection string, namespaceWithLibInjection string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider), pulumi.Timeouts(&pulumi.CustomTimeouts{Create: "20m", Update: "10m", Delete: "10m"}))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "mutated", k8sComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(k8sComponent))

	nsWithoutLibInjection, err := corev1.NewNamespace(e.Ctx(), namespaceWithoutLibInjection, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(namespaceWithoutLibInjection),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	nsWithLibInjection, err := corev1.NewNamespace(e.Ctx(), namespaceWithLibInjection, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(namespaceWithLibInjection),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	optsWithoutLibInjection := append(opts, pulumi.Parent(nsWithoutLibInjection), utils.PulumiDependsOn(nsWithoutLibInjection))
	optsWithLibInjection := append(opts, pulumi.Parent(nsWithLibInjection), utils.PulumiDependsOn(nsWithLibInjection))

	// openshift requires a non-default service account tied to the privileged scc
	saWithoutLibInjection, err := corev1.NewServiceAccount(e.Ctx(), "mutated-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("mutated-sa"),
			Namespace: pulumi.StringPtr(namespaceWithoutLibInjection),
		},
	}, optsWithoutLibInjection...)
	if err != nil {
		return nil, err
	}

	// create a RoleBinding to bind the new service account with the existing privileged scc
	if _, err := rbacv1.NewRoleBinding(e.Ctx(), "mutated-scc-binding", &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("mutated-scc-binding"),
			Namespace: pulumi.String(namespaceWithoutLibInjection),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:openshift:scc:hostaccess"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      saWithoutLibInjection.Metadata.Name().Elem(),
				Namespace: pulumi.String(namespaceWithoutLibInjection),
			},
		},
	}, optsWithoutLibInjection...); err != nil {
		return nil, err
	}

	saWithLibInjection, err := corev1.NewServiceAccount(e.Ctx(), "mutated-with-lib-sa", &corev1.ServiceAccountArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.StringPtr("mutated-with-lib-sa"),
			Namespace: pulumi.StringPtr(namespaceWithLibInjection),
		},
	}, optsWithLibInjection...)
	if err != nil {
		return nil, err
	}

	// create a RoleBinding to bind the new service account with the existing privileged scc
	if _, err := rbacv1.NewRoleBinding(e.Ctx(), "mutated-with-lib-scc-binding", &rbacv1.RoleBindingArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("mutated-with-lib-scc-binding"),
			Namespace: pulumi.String(namespaceWithLibInjection),
		},
		RoleRef: &rbacv1.RoleRefArgs{
			ApiGroup: pulumi.String("rbac.authorization.k8s.io"),
			Kind:     pulumi.String("ClusterRole"),
			Name:     pulumi.String("system:openshift:scc:hostaccess"),
		},
		Subjects: rbacv1.SubjectArray{
			rbacv1.SubjectArgs{
				Kind:      pulumi.String("ServiceAccount"),
				Name:      saWithLibInjection.Metadata.Name().Elem(),
				Namespace: pulumi.String(namespaceWithLibInjection),
			},
		},
	}, optsWithLibInjection...); err != nil {
		return nil, err
	}

	if err = k8sDeploymentWithoutLibInjection(e, namespaceWithoutLibInjection, "mutated", saWithoutLibInjection, optsWithoutLibInjection...); err != nil {
		return nil, err
	}
	if err = k8sDeploymentWithLibInjection(e, namespaceWithLibInjection, "mutated-with-lib-annotation", true, saWithLibInjection, optsWithLibInjection...); err != nil {
		return nil, err
	}
	if err = k8sDeploymentWithLibInjection(e, namespaceWithLibInjection, "mutated-with-auto-detected-language", false, saWithLibInjection, optsWithLibInjection...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}

func k8sDeploymentWithoutLibInjection(e config.Env, namespace string, name string, sa *corev1.ServiceAccount, opts ...pulumi.ResourceOption) error {
	_, err := appsv1.NewDeployment(e.Ctx(), name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(namespace),
			Labels:    pulumi.StringMap{"app": pulumi.String(name)},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{"app": pulumi.String(name)},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Annotations: pulumi.StringMap{
						"openshift.io/required-scc": pulumi.String("hostaccess"),
					},
					Labels: pulumi.StringMap{
						"app":                             pulumi.String(name),
						"admission.datadoghq.com/enabled": pulumi.String("true"),
						"tags.datadoghq.com/env":          pulumi.String("e2e"),
						"tags.datadoghq.com/service":      pulumi.String(name),
						"tags.datadoghq.com/version":      pulumi.String("v0.0.1"),
					},
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name:  pulumi.String(name),
							Image: pulumi.String("ghcr.io/datadog/apps-mutated:" + apps.Version),
						},
					},
				},
			},
		},
	}, opts...)

	return err
}

func k8sDeploymentWithLibInjection(e config.Env, namespace string, name string, withLibAnnotation bool, sa *corev1.ServiceAccount, opts ...pulumi.ResourceOption) error {
	annotations := pulumi.StringMap{
		"openshift.io/required-scc": pulumi.String("hostaccess"),
	}
	if withLibAnnotation {
		annotations["admission.datadoghq.com/python-lib.version"] = pulumi.String("v2.7.3")
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(name),
			Namespace: pulumi.String(namespace),
			Labels:    pulumi.StringMap{"app": pulumi.String(name)},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{"app": pulumi.String(name)},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app":                             pulumi.String(name),
						"admission.datadoghq.com/enabled": pulumi.String("true"),
						"tags.datadoghq.com/env":          pulumi.String("e2e"),
						"tags.datadoghq.com/service":      pulumi.String(name),
						"tags.datadoghq.com/version":      pulumi.String("v0.0.1"),
					},
					Annotations: annotations,
				},
				Spec: &corev1.PodSpecArgs{
					ServiceAccountName: sa.Metadata.Name().Elem(),
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name: pulumi.String(name),
							// Python is one of the languages supported by APM lib injection
							Image: pulumi.String("669783387624.dkr.ecr.us-east-1.amazonaws.com/dockerhub/library/python:3.12-slim-bullseye"),
							Command: pulumi.ToStringArray([]string{
								"python", "-c", "while True: import time; time.sleep(60)",
							}),
						},
					},
				},
			},
		},
	}, opts...); err != nil {
		return err
	}

	return nil
}
