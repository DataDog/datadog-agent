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

// Images holds the images of an Agent installation, those of a version. An empty field
// is not set: the Helm installation then falls back to its default images, which are the
// images of the pipeline under test when E2E_PIPELINE_ID and E2E_COMMIT_SHA are set (see
// components/datadog/agent/docker_image.go).
type Images struct {
	AgentFullImagePath        string
	ClusterAgentFullImagePath string
	AgentVersion              string
	ClusterAgentVersion       string
}

// merge overrides the fields of p with the non-empty fields of o.
func (p *Images) merge(o Images) {
	p.AgentFullImagePath = cmp.Or(o.AgentFullImagePath, p.AgentFullImagePath)
	p.ClusterAgentFullImagePath = cmp.Or(o.ClusterAgentFullImagePath, p.ClusterAgentFullImagePath)
	p.AgentVersion = cmp.Or(o.AgentVersion, p.AgentVersion)
	p.ClusterAgentVersion = cmp.Or(o.ClusterAgentVersion, p.ClusterAgentVersion)
}

// RunParams collects the parameters of the benchmarkeks scenario: the images of the
// installations of VariantA and VariantB.
type RunParams struct {
	a Images
	b Images
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

// ParamsFromEnvironment maps the image config keys of the environment to scenario
// parameters, then applies opts on top of them, so that an explicit option takes precedence
// over its config key. The images of the baseline version go to the installation of
// VariantA, those of the comparison version to the installation of VariantB.
//
// The image keys live in the ddagent: config namespace and are specific to this scenario, so
// they are read here rather than through the shared config.Env interface — same pattern as
// scenarios/aws/integrations/{lustre,dell_powerflex}. The invoke task maps its --baseline-* /
// --comparison-* flags onto them (see tasks/e2e_framework/aws/benchmarkeks.py), and e2e tests
// can set them with -c ddagent:<key>=<value>.
func ParamsFromEnvironment(e aws.Environment, opts ...RunOption) *RunParams {
	return GetRunParams(append([]RunOption{
		WithVariantA(imagesFromConfig(e, "baseline")),
		WithVariantB(imagesFromConfig(e, "comparison")),
	}, opts...)...)
}

// imagesFromConfig reads the ddagent:<version>FullImagePath,
// ddagent:<version>ClusterAgentFullImagePath, ddagent:<version>Version and
// ddagent:<version>ClusterAgentVersion config keys.
func imagesFromConfig(e aws.Environment, version string) Images {
	return Images{
		AgentFullImagePath:        e.AgentConfig.Get(version + "FullImagePath"),
		ClusterAgentFullImagePath: e.AgentConfig.Get(version + "ClusterAgentFullImagePath"),
		AgentVersion:              e.AgentConfig.Get(version + "Version"),
		ClusterAgentVersion:       e.AgentConfig.Get(version + "ClusterAgentVersion"),
	}
}

// WithVariantA sets the images of the Agent installation of VariantA. Only the non-empty
// fields of images are applied.
func WithVariantA(images Images) RunOption {
	return func(params *RunParams) error {
		params.a.merge(images)
		return nil
	}
}

// WithVariantB sets the images of the Agent installation of VariantB. Only the non-empty
// fields of images are applied.
func WithVariantB(images Images) RunOption {
	return func(params *RunParams) error {
		params.b.merge(images)
		return nil
	}
}
