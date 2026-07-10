// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package powershell

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	yaml "go.yaml.in/yaml/v2"
)

// allowlistVersion is the only supported allowlist schema version.
const allowlistVersion = 1

// allowedParam constrains a single cmdlet parameter: whether it is required and,
// if present, what values are acceptable (either an exact set or a regex).
type allowedParam struct {
	Required      bool     `yaml:"required"`
	AllowedValues []string `yaml:"allowed_values"`
	Pattern       string   `yaml:"pattern"`

	compiledPattern *regexp.Regexp
}

// allowedCmdlet is the policy for a single Get-* cmdlet.
type allowedCmdlet struct {
	Module     string                  `yaml:"module"`
	Parameters map[string]allowedParam `yaml:"parameters"`
}

// allowlist is the admin-owned policy of which cmdlets and parameters may run.
type allowlist struct {
	Version        int                      `yaml:"version"`
	AllowedCmdlets map[string]allowedCmdlet `yaml:"allowed_cmdlets"`
}

// parseAllowlist unmarshals and validates the admin allowlist. It fails closed:
// any structural problem returns an error and the caller must run nothing.
func parseAllowlist(data []byte) (*allowlist, error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, errors.New("allowlist is empty")
	}

	var a allowlist
	if err := yaml.UnmarshalStrict(data, &a); err != nil {
		return nil, fmt.Errorf("could not parse allowlist: %w", err)
	}
	if a.Version != allowlistVersion {
		return nil, fmt.Errorf("unsupported allowlist version %d (expected %d)", a.Version, allowlistVersion)
	}
	if len(a.AllowedCmdlets) == 0 {
		return nil, errors.New("allowlist has no allowed_cmdlets")
	}

	for name, cmd := range a.AllowedCmdlets {
		if err := validateGetCmdletName(name); err != nil {
			return nil, fmt.Errorf("allowlist entry %q: %w", name, err)
		}
		for pName, p := range cmd.Parameters {
			if err := validateIdentifier("parameter", pName); err != nil {
				return nil, fmt.Errorf("allowlist entry %q: %w", name, err)
			}
			if p.Pattern != "" {
				re, err := regexp.Compile(p.Pattern)
				if err != nil {
					return nil, fmt.Errorf("allowlist entry %q parameter %q: invalid pattern: %w", name, pName, err)
				}
				p.compiledPattern = re
				cmd.Parameters[pName] = p
			}
		}
	}
	return &a, nil
}

// validateInstance checks a parsed instance config against the allowlist. It
// rejects any cmdlet, parameter, or value that policy does not permit — this is
// the primary enforcement layer.
func (a *allowlist) validateInstance(inst *instanceConfig) error {
	if err := a.validateCmdletUse(inst.Cmdlet, inst.Filters); err != nil {
		return err
	}
	// tag_queries invoke additional cmdlets; those must be allowlisted too.
	// They take no user-supplied parameters, so only the cmdlet name is checked.
	for i := range inst.TagQueries {
		if err := a.validateCmdletUse(inst.TagQueries[i].TargetCmdlet, nil); err != nil {
			return fmt.Errorf("tag_queries: %w", err)
		}
	}
	return nil
}

// validateCmdletUse verifies a cmdlet is allowlisted and that the given filters
// satisfy the policy for that cmdlet.
func (a *allowlist) validateCmdletUse(cmdlet string, filters []filterEntry) error {
	policy, ok := a.AllowedCmdlets[cmdlet]
	if !ok {
		return fmt.Errorf("cmdlet %q is not in the allowlist", cmdlet)
	}

	provided := make(map[string]struct{}, len(filters))
	for i := range filters {
		f := filters[i]
		provided[f.Name] = struct{}{}
		p, ok := policy.Parameters[f.Name]
		if !ok {
			return fmt.Errorf("parameter %q of cmdlet %q is not permitted by the allowlist", f.Name, cmdlet)
		}
		if err := p.validateValue(scalarToString(f.Value)); err != nil {
			return fmt.Errorf("cmdlet %q parameter %q: %w", cmdlet, f.Name, err)
		}
	}

	for pName, p := range policy.Parameters {
		if !p.Required {
			continue
		}
		if _, ok := provided[pName]; !ok {
			return fmt.Errorf("cmdlet %q requires parameter %q", cmdlet, pName)
		}
	}
	return nil
}

// validateValue checks a stringified parameter value against the policy's
// allowed_values / pattern constraints.
func (p *allowedParam) validateValue(value string) error {
	if len(p.AllowedValues) > 0 {
		for _, v := range p.AllowedValues {
			if v == value {
				return nil
			}
		}
		return fmt.Errorf("value %q is not in allowed_values", value)
	}
	if p.compiledPattern != nil {
		if !p.compiledPattern.MatchString(value) {
			return fmt.Errorf("value %q does not match pattern %q", value, p.Pattern)
		}
	}
	return nil
}
