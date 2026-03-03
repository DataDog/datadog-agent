// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package operatorparams

import (
	"fmt"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/config"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
)

const (
	DatadogHelmRepo = "https://helm.datadoghq.com"
)

type Params struct {
	// OperatorFullImagePath is the full path of the operator image to use.
	OperatorFullImagePath string
	// Namespace is the namespace to deploy the operator to.
	Namespace string
	// HelmValues is the Helm values to use for the operator installation.
	HelmValues pulumi.AssetOrArchiveArray
	// HelmRepoURL is the Helm repo URL to use for the operator installation.
	HelmRepoURL string
	// HelmChartPath is the Helm chart path to use for the operator installation.
	HelmChartPath string
	// PulumiResourceOptions is a list of resources to depend on.
	PulumiResourceOptions []pulumi.ResourceOption
}

type Option = func(*Params) error

func NewParams(e config.Env, options ...Option) (*Params, error) {
	version := &Params{
		Namespace:     "datadog",
		HelmRepoURL:   DatadogHelmRepo,
		HelmChartPath: "datadog-operator",
	}

	if e.PipelineID() != "" && e.CommitSHA() != "" {
		options = append(options, WithOperatorFullImagePath(utils.BuildDockerImagePath(fmt.Sprintf("%s/operator", e.InternalRegistry()), fmt.Sprintf("%s-%s", e.PipelineID(), e.CommitSHA()))))
	}

	if e.OperatorLocalChartPath() != "" {
		options = append(options, WithHelmChartPath(e.OperatorLocalChartPath()))
		options = append(options, WithHelmRepoURL(""))
	}

	return common.ApplyOption(version, options)
}

// WithNamespace sets the namespace to deploy the agent to.
func WithNamespace(namespace string) func(*Params) error {
	return func(p *Params) error {
		p.Namespace = namespace
		return nil
	}
}

// WithOperatorFullImagePath sets the namespace to deploy the agent to.
func WithOperatorFullImagePath(path string) func(*Params) error {
	return func(p *Params) error {
		p.OperatorFullImagePath = path
		return nil
	}
}

// WithHelmValues adds helm values to the operator installation. If used several times, the helm values are merged together
// If the same values is defined several times the latter call will override the previous one.
// Accepts a string for single-line values (e.g. installCRDs: true) or a string literal in yaml format
// for multi-line values
func WithHelmValues(values string) func(*Params) error {
	return func(p *Params) error {
		p.HelmValues = append(p.HelmValues, pulumi.NewStringAsset(values))
		return nil
	}
}

// WithHelmRepoURL specifies the remote Helm repo URL to use for the datadog-operator installation.
func WithHelmRepoURL(repoURL string) func(*Params) error {
	return func(p *Params) error {
		p.HelmRepoURL = repoURL
		return nil
	}
}

// WithHelmChartPath specifies the remote chart name or local chart path to use for the datadog-operator installation.
func WithHelmChartPath(chartPath string) func(*Params) error {
	return func(p *Params) error {
		p.HelmChartPath = chartPath
		return nil
	}
}

// WithPulumiResourceOptions sets the resources to depend on.
func WithPulumiResourceOptions(resources ...pulumi.ResourceOption) func(*Params) error {
	return func(p *Params) error {
		p.PulumiResourceOptions = append(p.PulumiResourceOptions, resources...)
		return nil
	}
}
