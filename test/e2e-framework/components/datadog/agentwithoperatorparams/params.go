// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package agentwithoperatorparams

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/DataDog/datadog-agent/test/e2e-framework/common"
	"github.com/DataDog/datadog-agent/test/e2e-framework/common/utils"
	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/fakeintake"
)

type Params struct {
	PulumiResourceOptions []pulumi.ResourceOption

	Namespace  string
	FakeIntake *fakeintake.Fakeintake
	DDAConfig  DDAConfig
}

type Option = func(*Params) error

func NewParams(options ...Option) (*Params, error) {
	version := &Params{
		Namespace: "datadog",
		DDAConfig: DDAConfig{
			Name: "dda",
		},
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

// WithPulumiResourceOptions sets the resources to depend on.
func WithPulumiResourceOptions(resources ...pulumi.ResourceOption) func(*Params) error {
	return func(p *Params) error {
		p.PulumiResourceOptions = append(p.PulumiResourceOptions, resources...)
		return nil
	}
}

// WithDDAConfig configures the DatadogAgent custom resource.
func WithDDAConfig(config DDAConfig) func(*Params) error {
	return func(p *Params) error {
		p.DDAConfig = config
		return nil
	}
}

// WithFakeIntake configures the Agent to use the given fake intake.
func WithFakeIntake(fakeintake *fakeintake.Fakeintake) func(*Params) error {
	return func(p *Params) error {
		p.PulumiResourceOptions = append(p.PulumiResourceOptions, utils.PulumiDependsOn(fakeintake))
		p.FakeIntake = fakeintake
		return nil
	}
}

// DDAConfig is the DatadogAgent custom resource configuration.
type DDAConfig struct {
	// Name of the DatadogAgent custom resource
	Name string `json:"name"`
	// YamlFilePath file path to the DatadogAgent custom resource YAML
	YamlFilePath string `json:"yamlFilePath,omitempty"`
	// YamlConfig is the YAML string of the DatadogAgent custom resource
	YamlConfig string `json:"YamlConfig,omitempty"`
	// MapConfig is the map representation of the DatadogAgent custom resource
	MapConfig map[string]interface{} `json:"MapConfig,omitempty"`
}
