// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package types

import (
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// InternalConfig is a wrapper around integration.Config that
// holds additional information for configuration reconciliation.
type InternalConfig struct {
	integration.Config
	matchingProgram MatchingProgram
}

// IsTemplate returns true if the config is a template with AD identifiers or CEL rules.
func (a *InternalConfig) IsTemplate() bool {
	// Defined matchingProgram implies a CEL matching rule exists
	return len(a.ADIdentifiers) > 0 || len(a.AdvancedADIdentifiers) > 0 || a.matchingProgram != nil
}

// IsMatched returns true if the given object matches the CEL program of the config.
func (a *InternalConfig) IsMatched(obj workloadfilter.Filterable) bool {
	// If there's no matching program, then the config already matches w/ AD identifiers
	if a.matchingProgram == nil {
		return true
	}
	return a.matchingProgram.IsMatched(obj)
}

// CreateAdvancedConfig creates an AdvancedConfig from the given integration.Config.
func CreateAdvancedConfig(conf integration.Config) (InternalConfig, error) {
	prg, err := createMatchingProgram(conf.CELSelector)
	if err != nil {
		return InternalConfig{}, err
	}
	return InternalConfig{
		Config:          conf,
		matchingProgram: prg,
	}, nil
}
