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
	"testing"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// Mock is a mock implementation of the delegatedauth.Component interface
type Mock struct {
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

	providers  map[[2]string][]delegatedauth.Provider
	directives map[[3]string]delegatedauth.Provider
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

// AddInstance calls the mock function if set, then registers a provider for the params so
// ProvidersFor can find it. The returned provider is pending (no credential) unless the test
// supplies its own via ProvidersForFunc.
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
	if m.providers == nil {
		m.providers = map[[2]string][]delegatedauth.Provider{}
	}
	configKey, destination := params.ProviderKey()
	if m.directives == nil {
		m.directives = map[[3]string]delegatedauth.Provider{}
	}
	dkey := [3]string{configKey, destination, params.Directive}
	if _, seen := m.directives[dkey]; !seen {
		m.directives[dkey] = p
	}
	key := [2]string{configKey, destination}
	m.providers[key] = append(m.providers[key], p)
	return p, nil
}

// ProvidersFor implements delegatedauth.Component.
func (m *Mock) ProvidersFor(configKey, destination string) []delegatedauth.Provider {
	if m.ProvidersForFunc != nil {
		return m.ProvidersForFunc(configKey, destination)
	}
	return m.providers[[2]string{configKey, destination}]
}

// ProviderForDirective implements delegatedauth.Component.
func (m *Mock) ProviderForDirective(configKey, destination, directive string) delegatedauth.Provider {
	return m.directives[[3]string{configKey, destination, directive}]
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
