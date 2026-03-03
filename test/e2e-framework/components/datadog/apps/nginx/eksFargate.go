// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package nginx

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	sidecar "github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agent/k8ssidecar"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/apps/nginx/k8s"
	componentskube "github.com/DataDog/datadog-agent/test/e2e-framework/components/kubernetes"
)

func EksFargateAppDefinition(e config.Env, kubeProvider *kubernetes.Provider, namespace string, clusterAgentToken pulumi.StringInput, opts ...pulumi.ResourceOption) (*componentskube.Workload, error) {
	opts = append(opts, pulumi.Provider(kubeProvider), pulumi.Parent(kubeProvider), pulumi.DeletedWith(kubeProvider))

	eksFargateComponent := &componentskube.Workload{}
	if err := e.Ctx().RegisterComponentResource("dd:apps", "nginx-fargate", eksFargateComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(eksFargateComponent))

	ns, err := corev1.NewNamespace(e.Ctx(), namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, utils.PulumiDependsOn(ns))

	serviceAccount, err := sidecar.NewServiceAccountWithClusterPermissions(e.Ctx(), namespace, e.AgentAPIKey(), clusterAgentToken, opts...)

	if err != nil {
		return nil, err
	}

	nginxManifest, err := k8s.NewNginxDeploymentManifest(
		namespace,
		80,
		k8s.WithLabels(map[string]string{
			"agent.datadoghq.com/sidecar": "fargate",
		}),
		k8s.WithAnnotations(map[string]string{
			"ad.datadoghq.com/nginx.logs": `[{"source": "sidecar", "service": "nginx-fargate"}]`,
		}),
		k8s.WithServiceAccount(serviceAccount))

	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/nginx-fargate", nginxManifest, opts...); err != nil {
		return nil, err
	}

	if _, err := corev1.NewService(e.Ctx(), namespace+"/nginx", k8s.NewNginxServiceManifest(namespace, 80), opts...); err != nil {
		return nil, err
	}

	nginxQueryManifest, err := k8s.NewNginxQueryDeploymentManifest(
		namespace,
		k8s.WithLabels(map[string]string{
			"agent.datadoghq.com/sidecar": "fargate",
		}),
		k8s.WithAnnotations(map[string]string{
			"ad.datadoghq.com/query.logs": `[{"source": "sidecar", "service": "nginx-query-fargate"}]`,
		}),
	)
	if err != nil {
		return nil, err
	}

	if _, err := appsv1.NewDeployment(e.Ctx(), namespace+"/nginx-query", nginxQueryManifest, opts...); err != nil {
		return nil, err
	}

	return eksFargateComponent, nil
}
