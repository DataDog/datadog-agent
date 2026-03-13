// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package discovery

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type serviceSpec struct {
	name      string
	image     string
	port      int
	env       map[string]string
	command   []string
	args      []string
	resources *corev1.ResourceRequirementsArgs
}

func K8sAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	k8sComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "discovery", k8sComponent, opts...); err != nil {
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

	services := []serviceSpec{
		{
			name:  "python-svc",
			image: "ghcr.io/datadog/apps-discovery-python-svc:" + apps.Version,
			port:  8082,
			env: map[string]string{
				"PORT":       "8082",
				"DD_SERVICE": "python-svc-dd",
				"DD_VERSION": "2.1",
				"DD_ENV":     "prod",
			},
		},
		{
			name:  "python-instrumented",
			image: "ghcr.io/datadog/apps-discovery-python-instrumented:" + apps.Version,
			port:  8083,
			env: map[string]string{
				"PORT":       "8083",
				"DD_SERVICE": "python-instrumented-dd",
			},
		},
		{
			name:  "node-json-server",
			image: "ghcr.io/datadog/apps-discovery-node-json-server:" + apps.Version,
			port:  8084,
			env: map[string]string{
				"PORT": "8084",
			},
		},
		{
			name:  "node-instrumented",
			image: "ghcr.io/datadog/apps-discovery-node-instrumented:" + apps.Version,
			port:  8085,
			env: map[string]string{
				"PORT": "8085",
			},
		},
		{
			name:  "rails-svc",
			image: "ghcr.io/datadog/apps-discovery-rails:" + apps.Version,
			port:  7777,
			env: map[string]string{
				"PORT": "7777",
			},
		},
	}

	for _, svc := range services {
		if err := createDeployment(e, namespace, svc, opts...); err != nil {
			return nil, err
		}
	}

	return k8sComponent, nil
}

func createDeployment(e config.Env, namespace string, svc serviceSpec, opts ...pulumi.ResourceOption) error {
	envVars := corev1.EnvVarArray{}
	for k, v := range svc.env {
		envVars = append(envVars, corev1.EnvVarArgs{
			Name:  pulumi.String(k),
			Value: pulumi.String(v),
		})
	}

	container := corev1.ContainerArgs{
		Name:  pulumi.String(svc.name),
		Image: pulumi.String(svc.image),
		Env:   envVars,
		Ports: corev1.ContainerPortArray{
			corev1.ContainerPortArgs{
				ContainerPort: pulumi.Int(svc.port),
				Protocol:      pulumi.String("TCP"),
			},
		},
		ReadinessProbe: &corev1.ProbeArgs{
			TcpSocket: &corev1.TCPSocketActionArgs{
				Port: pulumi.Int(svc.port),
			},
			InitialDelaySeconds: pulumi.Int(5),
			PeriodSeconds:       pulumi.Int(10),
		},
		LivenessProbe: &corev1.ProbeArgs{
			TcpSocket: &corev1.TCPSocketActionArgs{
				Port: pulumi.Int(svc.port),
			},
			InitialDelaySeconds: pulumi.Int(15),
			PeriodSeconds:       pulumi.Int(20),
		},
	}

	if svc.resources != nil {
		container.Resources = svc.resources
	}

	if len(svc.command) > 0 {
		cmd := pulumi.StringArray{}
		for _, c := range svc.command {
			cmd = append(cmd, pulumi.String(c))
		}
		container.Command = cmd
	}

	if len(svc.args) > 0 {
		args := pulumi.StringArray{}
		for _, a := range svc.args {
			args = append(args, pulumi.String(a))
		}
		container.Args = args
	}

	_, err := appsv1.NewDeployment(e.Ctx(), svc.name, &appsv1.DeploymentArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(svc.name),
			Namespace: pulumi.String(namespace),
			Labels: pulumi.StringMap{
				"app": pulumi.String(svc.name),
			},
		},
		Spec: &appsv1.DeploymentSpecArgs{
			Replicas: pulumi.Int(1),
			Selector: &metav1.LabelSelectorArgs{
				MatchLabels: pulumi.StringMap{
					"app": pulumi.String(svc.name),
				},
			},
			Template: &corev1.PodTemplateSpecArgs{
				Metadata: &metav1.ObjectMetaArgs{
					Labels: pulumi.StringMap{
						"app": pulumi.String(svc.name),
					},
				},
				Spec: &corev1.PodSpecArgs{
					Containers: corev1.ContainerArray{container},
				},
			},
		},
	}, opts...)

	return err
}
