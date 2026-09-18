// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package config

import (
	"github.com/DataDog/datadog-agent/test/e2e-framework/cmd/internal/configschema"
	"go.yaml.in/yaml/v3"
)

func parseBuild(n *yaml.Node) (*BuildSelection, error) {
	members, err := configschema.Mapping(n, "agent.build")
	if err != nil {
		return nil, err
	}
	provider := members["provider"]
	if provider == nil || provider.Tag != "!!str" || provider.Value == "" {
		return nil, errf("agent.build.provider", "expected a nonempty string")
	}
	s := &BuildSelection{Provider: provider.Value}
	for key, value := range members {
		if key == "provider" {
			continue
		}
		if key != s.Provider {
			return nil, errf("agent.build."+key, "section does not match provider %q", s.Provider)
		}
		s.SectionNode = value
		s.Section, err = configschema.Encode(value)
		if err != nil {
			return nil, err
		}
	}
	return s, nil
}
