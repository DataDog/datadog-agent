// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package argorollouts

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
)

type Params struct {
	HelmValues HelmValues
	Version    string
	Namespace  string
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	params := &Params{
		Namespace: "argo-rollout",
	}
	return common.ApplyOption(params, options)
}

func WithHelmValues(values HelmValues) Option {
	return func(p *Params) error {
		p.HelmValues = values
		return nil
	}
}

func WithVersion(version string) Option {
	return func(p *Params) error {
		p.Version = version
		return nil
	}
}

func WithNamespace(namespace string) Option {
	return func(p *Params) error {
		p.Namespace = namespace
		return nil
	}
}

type HelmComponent struct {
	pulumi.ResourceState

	ArgoRolloutsHelmReleaseStatus helm.ReleaseStatusOutput
}
