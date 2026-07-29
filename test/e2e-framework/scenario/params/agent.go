// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package params holds reusable, tagged scenario parameter components (agent,
// fakeintake, …) that embed into a scenario's canonical params struct.
package params

import (
	"fmt"
	"os"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
)

// AgentParams is the reusable agent-configuration component. Its tagged fields
// cover at least the create-vm agent surface and convert to agentparams.Option.
type AgentParams struct {
	Version    string `scenario:"name=agent-version,help=Agent version (e.g. 7.42.0~rc.1-1); empty installs latest"`
	Flavor     string `scenario:"name=agent-flavor,help=Agent package flavor,enum=datadog-agent|datadog-fips-agent"`
	ConfigPath string `scenario:"name=agent-config-path,help=Path to a datadog.yaml whose contents are applied"`
	PipelineID string `scenario:"name=pipeline-id,help=GitLab pipeline build to install"`
	Install    bool   `scenario:"name=install-agent,default=true,help=Install the agent"`

	// AdvancedOptions is a Go-only escape hatch for the full agentparams surface.
	AdvancedOptions []agentparams.Option `scenario:"-"`
}

// ToOptions converts the component to agentparams options.
func (a AgentParams) ToOptions() ([]agentparams.Option, error) {
	var opts []agentparams.Option
	if a.Version != "" {
		opts = append(opts, agentparams.WithVersion(a.Version))
	}
	if a.Flavor != "" {
		opts = append(opts, agentparams.WithFlavor(a.Flavor))
	}
	if a.PipelineID != "" {
		opts = append(opts, agentparams.WithPipeline(a.PipelineID))
	}
	if a.ConfigPath != "" {
		content, err := os.ReadFile(a.ConfigPath)
		if err != nil {
			return nil, fmt.Errorf("reading agent-config-path: %w", err)
		}
		opts = append(opts, agentparams.WithAgentConfig(string(content)))
	}
	opts = append(opts, a.AdvancedOptions...)
	return opts, nil
}
