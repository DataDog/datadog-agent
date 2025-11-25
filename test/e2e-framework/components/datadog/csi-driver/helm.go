// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package csidriver

import (
	kubeHelm "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
)

type HelmComponent struct {
	pulumi.ResourceState

	CSIHelmReleaseStatus kubeHelm.ReleaseStatusOutput
}

type HelmValues pulumi.Map

type Params struct {
	HelmValues HelmValues
	Version    string
}

const (
	DatadogHelmRepo = "https://helm.datadoghq.com"
	CSINamespace    = "datadog-csi"
)

func NewHelmInstallation(e config.Env, params *Params, opts ...pulumi.ResourceOption) (*HelmComponent, error) {

	helmComponent := &HelmComponent{}
	if err := e.Ctx().RegisterComponentResource("dd:csi", "datadog-csi", helmComponent, opts...); err != nil {
		return nil, err
	}

	opts = append(opts, pulumi.Parent(helmComponent))
	csiBase, err := helm.NewInstallation(e, helm.InstallArgs{
		RepoURL:     DatadogHelmRepo,
		ChartName:   "datadog-csi-driver",
		InstallName: "datadog-csi",
		Namespace:   CSINamespace,
		Values:      pulumi.Map(params.HelmValues),
		Version:     pulumi.StringPtr(params.Version),
	}, opts...)
	if err != nil {
		return nil, err
	}

	helmComponent.CSIHelmReleaseStatus = csiBase.Status
	resourceOutputs := pulumi.Map{
		"CSIBaseHelmReleaseStatus": csiBase.Status,
	}

	if err := e.Ctx().RegisterResourceOutputs(helmComponent, resourceOutputs); err != nil {
		return nil, err
	}

	return helmComponent, nil
}
