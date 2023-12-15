// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rules holds rules related files
package rules

import (
	"github.com/hashicorp/go-multierror"
)

// DefaultPolicyName is the name of the default policy
// the default policy has a slightly privileged position when loading the rules
const DefaultPolicyName = "default.policy"

// PolicyProvider defines a rule provider
type PolicyProvider interface {
	LoadPolicies([]MacroFilter, []RuleFilter) ([]*Policy, *multierror.Error)
	SetOnNewPoliciesReadyCb(func())

	Start()
	Close() error

	// Type returns the type of policy provider, like 'directoryPolicyProvider'
	Type() string
}
