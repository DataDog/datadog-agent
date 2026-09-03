// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the delegatedauth component for testing
package mock

import (
	"context"
	"crypto/subtle"
	"net/http"
	"sync"
	"testing"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Mock is a mock implementation of the delegatedauth.Component interface
type Mock struct {
	mu sync.RWMutex

	AddInstanceFunc func(context.Context, delegatedauth.InstanceParams) error
	// ProvidersForFunc overrides ProvidersFor. When nil, providers returned from AddInstance are
	// indexed the same way the real component indexes them, so a test can wire a consumer without
	// standing up the whole component.
	ProvidersForFunc func(configKey, destination string) []delegatedauth.Provider

	// ProviderForInstanceFunc, when set, is called by AddInstance to obtain the provider for
	// each instance. When nil, AddInstance uses PendingProvider (the original behavior). This
	// lets a test return distinct providers per directive so "two orgs" tests prove real
	// separation rather than just bookkeeping.
	ProviderForInstanceFunc func(params delegatedauth.InstanceParams) delegatedauth.Provider

	providers map[[2]string][]registeredProvider
}

type registeredProvider struct {
	instanceKey string
	directive   string
	provider    delegatedauth.Provider
}

// StaticProvider is a Provider that always authorizes with a fixed key, for tests that only care
// that a consumer stamps what it is given.
type StaticProvider struct {
	Key string
}

// Authorize implements delegatedauth.Provider.
func (p StaticProvider) Authorize(h http.Header) bool {
	h.Set("DD-Api-Key", p.Key)
	return true
}

// Refresh implements delegatedauth.Provider.
func (StaticProvider) Refresh() bool { return false }

// PendingProvider is a Provider that never has a credential, for tests that exercise the
// buffer-until-resolved path.
type PendingProvider struct{}

// Authorize implements delegatedauth.Provider and always reports "no credential yet".
func (PendingProvider) Authorize(_ http.Header) bool { return false }

// Refresh implements delegatedauth.Provider. A pending provider has no background refresh.
func (PendingProvider) Refresh() bool { return false }

var _ delegatedauth.Component = (*Mock)(nil)

// Provides is the mock component output
type Provides struct {
	Comp delegatedauth.Component
}

// New creates a new mock delegatedauth component for testing
func New(_ testing.TB) delegatedauth.Component {
	return &Mock{}
}

// AddInstance calls the mock function if set, then registers a provider for the params.
// ProviderForInstanceFunc can supply a custom provider; the default is pending.
func (m *Mock) AddInstance(ctx context.Context, params delegatedauth.InstanceParams) (delegatedauth.Provider, error) {
	var err error
	if m.AddInstanceFunc != nil {
		err = m.AddInstanceFunc(ctx, params)
	}
	if err != nil {
		return nil, err
	}

	p := delegatedauth.Provider(PendingProvider{})
	if m.ProviderForInstanceFunc != nil {
		p = m.ProviderForInstanceFunc(params)
	}
	configKey, destination := params.ProviderKey()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.providers == nil {
		m.providers = map[[2]string][]registeredProvider{}
	}
	m.removeProviderLocked(params.APIKeyConfigKey)
	key := [2]string{configKey, destination}
	m.providers[key] = append(m.providers[key], registeredProvider{
		instanceKey: params.APIKeyConfigKey,
		directive:   params.Directive,
		provider:    p,
	})
	return p, nil
}

func (m *Mock) removeProviderLocked(instanceKey string) {
	for key, registered := range m.providers {
		kept := registered[:0]
		for _, candidate := range registered {
			if candidate.instanceKey != instanceKey {
				kept = append(kept, candidate)
			}
		}
		if len(kept) == 0 {
			delete(m.providers, key)
		} else {
			m.providers[key] = kept
		}
	}
}

// ProvidersFor implements delegatedauth.Component.
func (m *Mock) ProvidersFor(configKey, destination string) []delegatedauth.Provider {
	if m.ProvidersForFunc != nil {
		return m.ProvidersForFunc(configKey, destination)
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	registered := m.providers[[2]string{configKey, destination}]
	providers := make([]delegatedauth.Provider, len(registered))
	for i, candidate := range registered {
		providers[i] = candidate.provider
	}
	return providers
}

// ProviderForDirective implements delegatedauth.Component.
func (m *Mock) ProviderForDirective(configKey, destination, directive string) delegatedauth.Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, candidate := range m.providers[[2]string{configKey, destination}] {
		if candidate.directive == directive {
			return candidate.provider
		}
	}
	return nil
}

// RefreshFor implements delegatedauth.Component.
func (m *Mock) RefreshFor(configKey, destination, credential string) bool {
	for _, provider := range m.ProvidersFor(configKey, destination) {
		header := http.Header{}
		if provider.Authorize(header) && subtle.ConstantTimeCompare([]byte(header.Get("DD-Api-Key")), []byte(credential)) == 1 {
			return provider.Refresh()
		}
	}
	return false
}
