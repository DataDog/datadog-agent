// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package argorollouts

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
)

type HelmValues pulumi.Map

func NewHelmInstallation(e config.Env, params *Params, kubernetesProvider *kubernetes.Provider, opts ...pulumi.ResourceOption) (*HelmComponent, error) {
	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:argorollouts", "argorollouts", helmComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(helmComponent), pulumi.DeletedWith(kubernetesProvider), pulumi.Provider(kubernetesProvider))

	helmValues := pulumi.Map{}
	if params.HelmValues != nil {
		helmValues = pulumi.Map(params.HelmValues)
	}

	var version pulumi.StringPtrInput
	if params.Version != "" {
		version = pulumi.StringPtr(params.Version)
	}

	ns, err := corev1.NewNamespace(e.Ctx(), params.Namespace, &corev1.NamespaceArgs{
		Metadata: metav1.ObjectMetaArgs{
			Name: pulumi.String(params.Namespace),
		},
	}, opts...)
	if err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(ns))

	argoRollouts, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     "https://argoproj.github.io/argo-helm",
		ChartName:   "argo-rollouts",
		InstallName: "argo-rollouts",
		Namespace:   params.Namespace,
		Values:      helmValues,
		Version:     version,
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.ArgoRolloutsHelmReleaseStatus = argoRollouts.Status

	resourceOutputs := pulumi.Map{
		"ArgoRolloutsHelmReleaseStatus": argoRollouts.Status,
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}
