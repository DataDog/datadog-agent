// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package rules

import (
	"sync"
	"time"

	"github.com/hashicorp/go-multierror"
	"github.com/skydive-project/go-debouncer"
)

var (
	debounceDelay = 5 * time.Second
)

// PolicyLoader defines a policy loader
type PolicyLoader struct {
	sync.RWMutex

	Providers []PolicyProvider

	listeners []chan struct{}
	debouncer *debouncer.Debouncer
}

// LoadPolicies loads the policies
func (p *PolicyLoader) LoadPolicies() ([]*Policy, *multierror.Error) {
	p.RLock()
	defer p.RUnlock()

	var (
		errs          *multierror.Error
		allPolicies   []*Policy
		defaultPolicy *Policy
	)

	// use the provider in the order of insertion, keep the very last default policy
	for _, provider := range p.Providers {
		policies, err := provider.LoadPolicies()
		if err.ErrorOrNil() != nil {
			errs = multierror.Append(errs, err)
		}

		for _, policy := range policies {
			if policy.Name == DefaultPolicyName {
				defaultPolicy = policy
			} else {
				allPolicies = append(allPolicies, policy)
			}
		}
	}

	if defaultPolicy != nil {
		allPolicies = append([]*Policy{defaultPolicy}, allPolicies...)
	}

	return allPolicies, errs
}

// NewPolicyReady returns chan to listen new policy ready event
func (p *PolicyLoader) NewPolicyReady() <-chan struct{} {
	p.Lock()
	defer p.Unlock()

	ch := make(chan struct{})
	p.listeners = append(p.listeners, ch)
	return ch
}

func (p *PolicyLoader) onNewPoliciesReady() {
	p.debouncer.Call()
}

func (p *PolicyLoader) notifyListeners() {
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

// Close stops the loader
func (p *PolicyLoader) Close() {
	p.RLock()
	defer p.RUnlock()

	for _, ch := range p.listeners {
		close(ch)
	}

	p.debouncer.Stop()
}

// SetProviders set providers
func (p *PolicyLoader) SetProviders(providers []PolicyProvider) {
	p.Lock()
	defer p.Unlock()

	p.Providers = providers
	for _, provider := range providers {
		provider.SetOnNewPoliciesReadyCb(p.onNewPoliciesReady)
	}
}

// NewPolicyLoader returns a new loader
func NewPolicyLoader(providers ...PolicyProvider) *PolicyLoader {
	p := &PolicyLoader{}

	p.debouncer = debouncer.New(debounceDelay, p.notifyListeners)
	p.debouncer.Start()

	p.SetProviders(providers)

	return p
}
