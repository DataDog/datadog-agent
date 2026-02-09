// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package bundled contains bundled rules
package bundled

import (
	"sync"

	"github.com/hashicorp/go-multierror"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/seclog"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// PolicyProvider specify the policy provider for bundled policies
type PolicyProvider struct {
	sync.RWMutex
	cfg                  *config.RuntimeSecurityConfig
	onNewPoliciesReadyCb func(silent bool)
	// Map of SBOM PolicyDefs per workload key (image:tag)
	sbomPolicyDefs       map[string]*rules.PolicyDef
}

// NewPolicyProvider returns a new bundled policy provider
func NewPolicyProvider(cfg *config.RuntimeSecurityConfig) *PolicyProvider {
	return &PolicyProvider{
		cfg:            cfg,
		sbomPolicyDefs: make(map[string]*rules.PolicyDef),
	}
}

// LoadPolicies implements the PolicyProvider interface
func (p *PolicyProvider) LoadPolicies([]rules.MacroFilter, []rules.RuleFilter) ([]*rules.Policy, *multierror.Error) {
	p.RLock()
	defer p.RUnlock()

	// Start with base bundled rules
	policyDef := &rules.PolicyDef{
		Version: version.AgentVersion,
		Rules:   newBundledPolicyRules(p.cfg),
	}

	// Merge all SBOM-generated macros and rules from all workload keys
	for workloadKey, sbomPolicyDef := range p.sbomPolicyDefs {
		if sbomPolicyDef != nil {
			policyDef.Macros = append(policyDef.Macros, sbomPolicyDef.Macros...)
			policyDef.Rules = append(policyDef.Rules, sbomPolicyDef.Rules...)
			seclog.Debugf("Merged SBOM policy for workload %s: %d macros, %d rules",
				workloadKey, len(sbomPolicyDef.Macros), len(sbomPolicyDef.Rules))
		}
	}

	pInfo := &rules.PolicyInfo{
		Name:         "bundled_policy",
		Source:       "bundled",
		InternalType: rules.BundledPolicyType,
		IsInternal:   true,
	}

	policy, err := rules.LoadPolicyFromDefinition(pInfo, policyDef, nil, nil)
	if err != nil {
		return nil, multierror.Append(nil, err)
	}
	policy.SetInternalCallbackAction(RefreshUserCacheRuleID, RefreshSBOMRuleID)

	return []*rules.Policy{policy}, nil
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (p *PolicyProvider) SetOnNewPoliciesReadyCb(cb func(silent bool)) {
	p.Lock()
	defer p.Unlock()
	p.onNewPoliciesReadyCb = cb
}

// SetSBOMPolicyDef sets the SBOM-generated policy definition for a workload key and triggers a silent reload
func (p *PolicyProvider) SetSBOMPolicyDef(workloadKey string, policyDef *rules.PolicyDef) {
	p.Lock()
	p.sbomPolicyDefs[workloadKey] = policyDef
	cb := p.onNewPoliciesReadyCb
	p.Unlock()

	if cb != nil {
		// SBOM policy updates are silent - don't trigger heartbeat events
		cb(true)
	}
}

// RemoveSBOMPolicyDef removes the SBOM policy definition for a workload key
func (p *PolicyProvider) RemoveSBOMPolicyDef(workloadKey string) {
	p.Lock()
	delete(p.sbomPolicyDefs, workloadKey)
	cb := p.onNewPoliciesReadyCb
	p.Unlock()

	if cb != nil {
		// Trigger a silent reload to remove the policies
		cb(true)
	}
}

// Start implements the PolicyProvider interface
func (p *PolicyProvider) Start() {}

// Close implements the PolicyProvider interface
func (p *PolicyProvider) Close() error { return nil }

// Type implements the PolicyProvider interface
func (p *PolicyProvider) Type() string { return rules.PolicyProviderTypeBundled }
