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
	policyDef := &rules.PolicyDef{
		Version: version.AgentVersion,
		Rules:   newBundledPolicyRules(p.cfg),
	}

	policy, err := rules.LoadPolicyFromDefinition("bundled_policy", "bundled", policyDef, nil, nil)
	if err != nil {
		return nil, multierror.Append(nil, err)
	}
	policy.IsInternal = true
	policy.SetInternalCallbackAction(RefreshUserCacheRuleID, RefreshSBOMRuleID)

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
