// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"bytes"
	_ "embed"

	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/hashicorp/go-multierror"
)

//go:embed default.policy
var defaultPolicyContent []byte

// DefaultFallbackProvider specify the policy provider for embedded fallback default policies
type DefaultFallbackProvider struct {
}

// LoadPolicies implements the PolicyProvider interface
func (p *DefaultFallbackProvider) LoadPolicies(macroFilters []rules.MacroFilter, ruleFilters []rules.RuleFilter, hasAlreadyLoadedUserPolicies bool) ([]*rules.Policy, *multierror.Error) {
	// no need for the fallback
	if hasAlreadyLoadedUserPolicies {
		return nil, nil
	}

	reader := bytes.NewReader(defaultPolicyContent)
	policy, err := rules.LoadPolicy("embed_policy", "embed", reader, macroFilters, ruleFilters)

	merr := multierror.Append(nil, err)
	return []*rules.Policy{policy}, merr
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (p *DefaultFallbackProvider) SetOnNewPoliciesReadyCb(func()) {}

// Start implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Start() {}

// Close implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Close() error { return nil }

// Type implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Type() string { return rules.PolicyProviderTypeEmbedFallback }
