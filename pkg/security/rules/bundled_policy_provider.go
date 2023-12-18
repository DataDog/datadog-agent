// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// BundledPolicyProvider specify the policy provider for bundled policies
type BundledPolicyProvider struct {
	cfg *config.RuntimeSecurityConfig
}

// NewBundledPolicyProvider returns a new bundled policy provider
func NewBundledPolicyProvider(cfg *config.RuntimeSecurityConfig) *BundledPolicyProvider {
	return &BundledPolicyProvider{
		cfg: cfg,
	}
}

// LoadPolicies implements the PolicyProvider interface
func (p *BundledPolicyProvider) LoadPolicies([]rules.MacroFilter, []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
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
func (p *BundledPolicyProvider) SetOnNewPoliciesReadyCb(func()) {}

// Start implements the PolicyProvider interface
func (p *BundledPolicyProvider) Start() {}

// Close implements the PolicyProvider interface
func (p *BundledPolicyProvider) Close() error { return nil }

// Type implements the PolicyProvider interface
func (p *BundledPolicyProvider) Type() string { return rules.PolicyProviderTypeBundled }
