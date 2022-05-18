// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"fmt"

	"github.com/hashicorp/go-multierror"
)

// PolicyLoader defines a policy loader
type PolicyLoader struct {
	Providers []PolicyProvider
}

// LoadPolicies loads the policies
func (p *PolicyLoader) LoadPolicies() ([]*Policy, *multierror.Error) {
	var errs *multierror.Error
	var policies []*Policy

	for _, provider := range p.Providers {
		policy, err := provider.LoadPolicy()
		if err != nil {
			errs = multierror.Append(errs, err)
		}

		if policy != nil {
			policies = append(policies, policy)
		}
	}

	return policies, errs
}

func (p *PolicyLoader) OnPolicyChanged(policy *Policy) {
	fmt.Printf("AHAHA\n")
}

func NewPolicyLoader(providers []PolicyProvider) *PolicyLoader {
	p := &PolicyLoader{
		Providers: providers,
	}

	for _, provider := range providers {
		provider.SetOnPolicyChangedCb(p.OnPolicyChanged)
	}

	return p
}
