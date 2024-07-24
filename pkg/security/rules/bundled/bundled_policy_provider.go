// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bundled contains bundled rules
package bundled

import (
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// PolicyProvider specify the policy provider for bundled policies
type PolicyProvider struct {
	cfg *config.RuntimeSecurityConfig
}

// NewPolicyProvider returns a new bundled policy provider
func NewPolicyProvider(cfg *config.RuntimeSecurityConfig) *PolicyProvider {
	return &PolicyProvider{
		cfg: cfg,
	}
}

// LoadPolicies implements the PolicyProvider interface
func (p *PolicyProvider) LoadPolicies([]rules.MacroFilter, []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	bundledPolicyRules := newBundledPolicyRules(p.cfg)

	policy := &rules.Policy{}

	policy.Name = "bundled_policy"
	policy.Source = "bundled"
	policy.Version = version.AgentVersion
	policy.Rules = bundledPolicyRules
	policy.IsInternal = true

	for _, rule := range bundledPolicyRules {
		rule.Policy = policy
	}

	return []*rules.Policy{policy}, nil
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (p *PolicyProvider) SetOnNewPoliciesReadyCb(func()) {}

// Start implements the PolicyProvider interface
func (p *PolicyProvider) Start() {}

// Close implements the PolicyProvider interface
func (p *PolicyProvider) Close() error { return nil }

// Type implements the PolicyProvider interface
func (p *PolicyProvider) Type() string { return rules.PolicyProviderTypeBundled }
