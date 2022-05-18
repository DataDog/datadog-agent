// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"sync"

	"github.com/hashicorp/go-multierror"
)

// PolicyLoader defines a policy loader
type PolicyLoader struct {
	sync.RWMutex

	Providers []PolicyProvider
	listeners []chan struct{}
}

// LoadPolicies loads the policies
func (p *PolicyLoader) LoadPolicies() ([]*Policy, *multierror.Error) {
	p.RLock()
	defer p.RUnlock()

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

// NewPolicyReady returns chan to listen new policy ready event
func (p *PolicyLoader) NewPolicyReady() <-chan struct{} {
	p.Lock()
	defer p.Unlock()

	ch := make(chan struct{})
	p.listeners = append(p.listeners, ch)
	return ch
}

func (p *PolicyLoader) onNewPolicyReady(policy *Policy) {
	p.RLock()
	defer p.RUnlock()

	// TODO(safchain) debounce
	for _, ch := range p.listeners {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

// Stop the loader
func (p *PolicyLoader) Close() {
	p.RLock()
	defer p.RUnlock()

	for _, ch := range p.listeners {
		close(ch)
	}
}

// SetProviders set providers
func (p *PolicyLoader) SetProviders(providers []PolicyProvider) {
	p.Lock()
	defer p.Unlock()

	// first terminate the previous providers
	for _, provider := range p.Providers {
		provider.Stop()
	}

	p.Providers = providers

	for _, provider := range providers {
		provider.SetOnNewPolicyReadyCb(p.onNewPolicyReady)
	}
}

// NewPolicyLoader returns a new loader
func NewPolicyLoader(providers []PolicyProvider) *PolicyLoader {
	p := &PolicyLoader{}
	p.SetProviders(providers)
	return p
}
