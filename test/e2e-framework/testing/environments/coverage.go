// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package environments contains the definitions of the different environments that can be used in a test.
package environments

import "fmt"

// CoverageTargetSpec defines the name of the agent, the command to run to generate the coverage and if the coverage is required
type CoverageTargetSpec struct {
	AgentName       string
	CoverageCommand []string
	Required        bool
}

// CoverageBase provides shared coverage override logic. Embed it in any environment
// that implements Coverageable so the override setter and application come for free.
type CoverageBase struct {
	// requiredOverrides holds per-agent overrides keyed by AgentName.
	// A nil map means "use defaults for everything".
	requiredOverrides map[string]bool
}

// SetCoverageRequiredOverride records per-agent overrides for the Required field.
// Each key must match a CoverageTargetSpec.AgentName. Passing a nil map clears all overrides.
func (c *CoverageBase) SetCoverageRequiredOverride(overrides map[string]bool) {
	c.requiredOverrides = overrides
}

// applyCoverageOverrides mutates targets in place, replacing Required for any
// agent whose name appears in the override map.
func (c *CoverageBase) applyCoverageOverrides(targets []CoverageTargetSpec) {
	for i := range targets {
		if required, ok := c.requiredOverrides[targets[i].AgentName]; ok {
			targets[i].Required = required
		}
	}
}

func updateErrorOutput(target CoverageTargetSpec, outStr []string, errs []error, errorMessage string) ([]string, []error) {
	outStr = append(outStr, fmt.Sprintf("[WARN] %s: %s", target.AgentName, errorMessage))
	if target.Required {
		errs = append(errs, fmt.Errorf("[ERROR] %s: %s", target.AgentName, errorMessage))
	}
	return outStr, errs
}
