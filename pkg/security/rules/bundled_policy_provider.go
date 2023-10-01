// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/hashicorp/go-multierror"
)

const refreshUserCacheRuleID = "refresh_user_cache"

// BundledPolicyProvider specify the policy provider for bundled policies
type BundledPolicyProvider struct{}

// LoadPolicies implements the PolicyProvider interface
func (p *BundledPolicyProvider) LoadPolicies([]rules.MacroFilter, []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	policy := &rules.Policy{}

	refreshUserCacheAction := rules.ActionDefinition{
		InternalCallbackDefinition: &rules.InternalCallbackDefinition{},
	}

	policy.Name = "bundled_policy"
	policy.Source = "bundled"
	policy.Version = version.AgentVersion
	policy.Rules = []*rules.RuleDefinition{{
		ID:         refreshUserCacheRuleID,
		Expression: `rename.file.destination.path in [ "/etc/passwd", "/etc/group" ]`,
		Actions:    []rules.ActionDefinition{refreshUserCacheAction},
		Policy:     policy,
		Silent:     true,
	}}

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
