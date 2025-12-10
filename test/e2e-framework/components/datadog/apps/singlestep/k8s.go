// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package singlestep provides a scenario to use for Single Step Instrumentation based tests.
package singlestep

import (
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

// Namespace is a namespace which should be created as part of a scenario. Within the namespace, a list of
// apps can optionally be defined.
type Namespace struct {
	// Name is the name of the namespace to create.
	Name string
	// Labels are the Kubernetes labels to apply to the namespace.
	Labels map[string]string
	// Annotations are the Kubernetes annotations to apply to the namespace.
	Annotations map[string]string
	// Apps are a list of apps to create inside the namespace.
	Apps []App
}

// App is a demo application that should be deployed as part of a scenario.
type App struct {
	// Name is the name of the app, to be used in the deployment and service resources.
	Name string
	// Image is the container image for the app.
	Image string
	// Version is the version tag for the app.
	Version string
	// Port is the HTTP port the app will listen on.
	Port int
	// PodLabels are the Kubernetes labels to apply to the pod spec.
	PodLabels map[string]string
	// PodAnnotations are the Kubernetes annotations to apply to the pod spec.
	PodAnnotations map[string]string
}

// Scenario creates a list of namespaces, each containing a list of demo applications. These are dependent on
// the provided KubernetesAgent deployment.
func Scenario(e config.Env, kubeProvider *kubernetes.Provider, scenarioName string, namespaces []Namespace, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	// Register component.
	k8sComponent := &componentskube.Workload{}
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))
	err := e.Ctx().RegisterComponentResource("dd:apps", scenarioName, k8sComponent, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to register component: %w", err)
	}

	for _, namespace := range namespaces {
		// Create sub options.
		o := make([]pulumi.ResourceOption, len(opts))
		copy(o, opts)
		o = append(o, pulumi.Parent(k8sComponent))

		// Sanitize the input.
		if namespace.Annotations == nil {
			namespace.Annotations = map[string]string{}
		}
		if namespace.Labels == nil {
			namespace.Labels = map[string]string{}
		}

		// Create the namespace.
		ns, err := corev1.NewNamespace(e.Ctx(), namespace.Name, &corev1.NamespaceArgs{
			Metadata: metav1.ObjectMetaArgs{
				Name:        pulumi.String(namespace.Name),
				Labels:      pulumi.ToStringMap(namespace.Labels),
				Annotations: pulumi.ToStringMap(namespace.Annotations),
			},
		}, o...)
		if err != nil {
			return nil, fmt.Errorf("could not create namespace %s: %w", namespace.Name, err)
		}

		// Create the apps in the namespace.
		for _, app := range namespace.Apps {
			// Sanitize the input.
			if app.PodAnnotations == nil {
				app.PodAnnotations = map[string]string{}
			}
			if app.PodLabels == nil {
				app.PodLabels = map[string]string{}
			}

			// Create the deployment.
			o = append(o, utils.PulumiDependsOn(ns))
			app.PodLabels["app"] = app.Name
			_, err = appsv1.NewDeployment(e.Ctx(), fmt.Sprintf("%s:deployment:%s", namespace.Name, app.Name), &appsv1.DeploymentArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String(app.Name),
					Namespace: pulumi.String(namespace.Name),
					Labels:    pulumi.StringMap{"app": pulumi.String(app.Name)},
				},
				Spec: &appsv1.DeploymentSpecArgs{
					Replicas: pulumi.Int(1),
					Selector: &metav1.LabelSelectorArgs{
						MatchLabels: pulumi.StringMap{"app": pulumi.String(app.Name)},
					},
					Template: &corev1.PodTemplateSpecArgs{
						Metadata: &metav1.ObjectMetaArgs{
							Labels:      pulumi.ToStringMap(app.PodLabels),
							Annotations: pulumi.ToStringMap(app.PodAnnotations),
						},
						Spec: &corev1.PodSpecArgs{
							Containers: corev1.ContainerArray{
								corev1.ContainerArgs{
									Name:  pulumi.String(app.Name),
									Image: pulumi.String(fmt.Sprintf("%s:%s", app.Image, app.Version)),
									Env: &corev1.EnvVarArray{
										&corev1.EnvVarArgs{
											Name:  pulumi.String("DD_TRACE_DEBUG"),
											Value: pulumi.String("true"),
										},
										&corev1.EnvVarArgs{
											Name:  pulumi.String("DD_APM_INSTRUMENTATION_DEBUG"),
											Value: pulumi.String("true"),
										},
									},
									ReadinessProbe: &corev1.ProbeArgs{
										HttpGet: &corev1.HTTPGetActionArgs{
											Path: pulumi.String("/"),
											Port: pulumi.Int(app.Port),
										},
										InitialDelaySeconds: pulumi.Int(1),
										PeriodSeconds:       pulumi.Int(1),
										TimeoutSeconds:      pulumi.Int(1),
									},
								},
							},
						},
					},
				},
			}, o...)
			if err != nil {
				return nil, fmt.Errorf("could not create deployment %s: %w", app.Name, err)
			}

			// Create the service.
			_, err = corev1.NewService(e.Ctx(), fmt.Sprintf("%s:service:%s", namespace.Name, app.Name), &corev1.ServiceArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String(app.Name),
					Namespace: pulumi.String(namespace.Name),
				},
				Spec: &corev1.ServiceSpecArgs{
					Selector: pulumi.StringMap{
						"app": pulumi.String(app.Name),
					},
					Ports: &corev1.ServicePortArray{
						&corev1.ServicePortArgs{
							Port:       pulumi.Int(app.Port),
							TargetPort: pulumi.Int(app.Port),
						},
					},
				},
			}, o...)
			if err != nil {
				return nil, fmt.Errorf("could not create service %s: %w", app.Name, err)
			}
		}
	}

	return k8sComponent, nil
}
