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

const (
	PolicyProviderTypeDir = "directoryPolicyProvider"
	PolicySourceDir       = "file"
	PolicyProviderTypeRC  = "remoteConfigPolicyProvider"
	PolicySourceRC        = "remote-config"
)

var (
	debounceDelay = 5 * time.Second
)

// PolicyLoaderOpts options used during the loading
type PolicyLoaderOpts struct {
	MacroFilters []MacroFilter
	RuleFilters  []RuleFilter
}

// PolicyLoader defines a policy loader
type PolicyLoader struct {
	sync.RWMutex

	Providers []PolicyProvider

	listeners []chan struct{}
	debouncer *debouncer.Debouncer
}

// LoadPolicies gathers the policies in the correct precedence order and ensuring there's only 1 default policy.
// RC Default replaces Local Default and takes precedence above any other policies, and RC Custom takes precedence over Local Custom.
func (p *PolicyLoader) LoadPolicies(opts PolicyLoaderOpts) ([]*Policy, *multierror.Error) {
	p.RLock()
	defer p.RUnlock()

	var (
		errs          *multierror.Error
		allPolicies   []*Policy
		defaultPolicy *Policy
	)

	// use the providers in the order of insertion, keep the very last default policy
	p.remoteConfigProvidersFirst()
	for _, provider := range p.Providers {
		policies, err := provider.LoadPolicies(opts.MacroFilters, opts.RuleFilters)
		if err.ErrorOrNil() != nil {
			errs = multierror.Append(errs, err)
		}

		if policies == nil {
			continue
		}

		for _, policy := range policies {
			if policy.Name == DefaultPolicyName {
				if defaultPolicy == nil {
					defaultPolicy = policy
				} else if policy.Source == PolicySourceRC {
					defaultPolicy = policy // This ensures that a RC default policy always overwrites a local default policy, regardless of the order the providers were loaded
				}
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

// Rules from RC override local rules if they share the same ID, so the RC policy provider has to be first
func (p *PolicyLoader) remoteConfigProvidersFirst() {
	var remoteConfigProviders []PolicyProvider
	var dirProviders []PolicyProvider
	var unknownProviders []PolicyProvider

	for _, provider := range p.Providers {
		if provider.Type() == PolicyProviderTypeRC {
			remoteConfigProviders = append(remoteConfigProviders, provider)
		} else if provider.Type() == PolicyProviderTypeDir {
			dirProviders = append(dirProviders, provider)
		} else {
			unknownProviders = append(unknownProviders, provider)
		}
	}

	var allProviders []PolicyProvider
	allProviders = append(allProviders, remoteConfigProviders...)
	allProviders = append(allProviders, dirProviders...)
	allProviders = append(allProviders, unknownProviders...)

	p.Providers = allProviders
}
