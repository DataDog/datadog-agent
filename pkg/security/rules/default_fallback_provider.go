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

// DefaultFallbackProvider specify the policy provider for embedded fallback default policies
type DefaultFallbackProvider struct {
}

// LoadPolicies implements the PolicyProvider interface
func (p *DefaultFallbackProvider) LoadPolicies(_ []rules.MacroFilter, _ []rules.RuleFilter, hasAlreadyLoadedUserPolicies bool) ([]*rules.Policy, *multierror.Error) {
	// no need for the fallback
	if hasAlreadyLoadedUserPolicies {
		return nil, nil
	}

	policy := &rules.Policy{
		Name:    "embed_policy",
		Source:  "embed",
		Version: version.AgentVersion,
	}

	// TODO: use LoadPolicy as in policy_dir.go
	return []*rules.Policy{policy}, nil
}

// SetOnNewPoliciesReadyCb implements the PolicyProvider interface
func (p *DefaultFallbackProvider) SetOnNewPoliciesReadyCb(func()) {}

// Start implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Start() {}

// Close implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Close() error { return nil }

// Type implements the PolicyProvider interface
func (p *DefaultFallbackProvider) Type() string { return rules.PolicyProviderTypeEmbedFallback }
