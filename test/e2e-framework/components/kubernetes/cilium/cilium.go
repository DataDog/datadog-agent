// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package cilium

import (
	"reflect"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/helm"
)

type Params struct {
	HelmValues HelmValues
	Version    string
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	return common.ApplyOption(&Params{}, options)
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

type HelmComponent struct {
	pulumi.ResourceState

	CiliumHelmReleaseStatus helm.ReleaseStatusOutput
}

func boolValue(i pulumi.Input) bool {
	pv := reflect.ValueOf(i)
	if pv.Kind() == reflect.Ptr {
		if pv.IsNil() {
			return false
		}

		pv = pv.Elem()
	}

	return pv.Bool()
}

func (p *Params) hasKubeProxyReplacement() bool {
	if v, ok := p.HelmValues["kubeProxyReplacement"]; ok {
		return boolValue(v)
	}

	return false
}
