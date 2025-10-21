package mutatedbyadmissioncontroller

import (
	"github.com/DataDog/test-infra-definitions/common/config"

	"github.com/DataDog/test-infra-definitions/components/datadog/apps"
	componentskube "github.com/DataDog/test-infra-definitions/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
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
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

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

	if err = k8sDeploymentWithoutLibInjection(e, namespaceWithoutLibInjection, "mutated", append(opts, pulumi.Parent(nsWithoutLibInjection))...); err != nil {
		return nil, err
	}
	if err = k8sDeploymentWithLibInjection(e, namespaceWithLibInjection, "mutated-with-lib-annotation", true, append(opts, pulumi.Parent(nsWithLibInjection))...); err != nil {
		return nil, err
	}
	if err = k8sDeploymentWithLibInjection(e, namespaceWithLibInjection, "mutated-with-auto-detected-language", false, append(opts, pulumi.Parent(nsWithLibInjection))...); err != nil {
		return nil, err
	}

	return k8sComponent, nil
}

func k8sDeploymentWithoutLibInjection(e config.Env, namespace string, name string, opts ...pulumi.ResourceOption) error {
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
					Labels: pulumi.StringMap{
						"app":                             pulumi.String(name),
						"admission.datadoghq.com/enabled": pulumi.String("true"),
						"tags.datadoghq.com/env":          pulumi.String("e2e"),
						"tags.datadoghq.com/service":      pulumi.String(name),
						"tags.datadoghq.com/version":      pulumi.String("v0.0.1"),
					},
				},
				Spec: &corev1.PodSpecArgs{
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

func k8sDeploymentWithLibInjection(e config.Env, namespace string, name string, withLibAnnotation bool, opts ...pulumi.ResourceOption) error {
	annotations := pulumi.StringMap{}
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
					Containers: corev1.ContainerArray{
						corev1.ContainerArgs{
							Name: pulumi.String(name),
							// Python is one of the languages supported by APM lib injection
							Image: pulumi.String("public.ecr.aws/docker/library/python:3.12-slim"), // TODO: Change to using private mirror
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
