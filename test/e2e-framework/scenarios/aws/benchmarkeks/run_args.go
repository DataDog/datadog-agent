// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package benchmarkeks

import (
	"cmp"
	"fmt"

	"github.com/DataDog/datadog-agent/test/e2e-framework/resources/aws"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/utils/optional"
)

// VariantParams holds the images of the Agent installation of a variant. An empty field
// is not set: the Helm installation then falls back to its default images, which are the
// images of the pipeline under test when E2E_PIPELINE_ID and E2E_COMMIT_SHA are set (see
// components/datadog/agent/docker_image.go).
type VariantParams struct {
	AgentFullImagePath        string
	ClusterAgentFullImagePath string
	AgentVersion              string
	ClusterAgentVersion       string
}

// merge overrides the fields of p with the non-empty fields of o.
func (p *VariantParams) merge(o VariantParams) {
	p.AgentFullImagePath = cmp.Or(o.AgentFullImagePath, p.AgentFullImagePath)
	p.ClusterAgentFullImagePath = cmp.Or(o.ClusterAgentFullImagePath, p.ClusterAgentFullImagePath)
	p.AgentVersion = cmp.Or(o.AgentVersion, p.AgentVersion)
	p.ClusterAgentVersion = cmp.Or(o.ClusterAgentVersion, p.ClusterAgentVersion)
}

// RunParams collects the parameters of the benchmarkeks scenario.
type RunParams struct {
	baseline   VariantParams
	comparison VariantParams
}

// RunOption is a function that modifies the RunParams.
type RunOption = func(*RunParams) error

// GetRunParams returns the scenario parameters set by opts, without reading the config keys.
func GetRunParams(opts ...RunOption) *RunParams {
	params := &RunParams{}
	if err := optional.ApplyOptions(params, opts); err != nil {
		panic(fmt.Errorf("unable to apply RunOption, err: %w", err))
	}
	return params
}

// ParamsFromEnvironment maps the per-variant config keys of the environment to scenario
// parameters, then applies opts on top of them, so that an explicit option takes precedence
// over its config key.
//
// The per-variant image keys live in the ddagent: config namespace and are specific to this
// scenario, so they are read here rather than through the shared config.Env interface — same
// pattern as scenarios/aws/integrations/{lustre,dell_powerflex}. The invoke task maps its
// --baseline-* / --comparison-* flags onto them (see tasks/e2e_framework/aws/benchmarkeks.py),
// and e2e tests can set them with -c ddagent:<key>=<value>.
func ParamsFromEnvironment(e aws.Environment, opts ...RunOption) *RunParams {
	return GetRunParams(append([]RunOption{
		WithBaseline(variantParamsFromConfig(e, Baseline)),
		WithComparison(variantParamsFromConfig(e, Comparison)),
	}, opts...)...)
}

// variantParamsFromConfig reads the ddagent:<variant>FullImagePath,
// ddagent:<variant>ClusterAgentFullImagePath, ddagent:<variant>Version and
// ddagent:<variant>ClusterAgentVersion config keys.
func variantParamsFromConfig(e aws.Environment, variant string) VariantParams {
	return VariantParams{
		AgentFullImagePath:        e.AgentConfig.Get(variant + "FullImagePath"),
		ClusterAgentFullImagePath: e.AgentConfig.Get(variant + "ClusterAgentFullImagePath"),
		AgentVersion:              e.AgentConfig.Get(variant + "Version"),
		ClusterAgentVersion:       e.AgentConfig.Get(variant + "ClusterAgentVersion"),
	}
}

// WithBaseline sets the images of the baseline Agent installation. Only the non-empty
// fields of variant are applied.
func WithBaseline(variant VariantParams) RunOption {
	return func(params *RunParams) error {
		params.baseline.merge(variant)
		return nil
	}
}

// WithComparison sets the images of the comparison Agent installation. Only the
// non-empty fields of variant are applied.
func WithComparison(variant VariantParams) RunOption {
	return func(params *RunParams) error {
		params.comparison.merge(variant)
		return nil
	}
}
