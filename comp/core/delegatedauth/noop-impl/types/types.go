// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides the types for the noop implementation of the delegated auth component
package types

import (
	"context"
	"net/http"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
)

// DelegatedAuthNoop is a noop implementation of the delegated auth component
type DelegatedAuthNoop struct{}

var _ delegatedauth.Component = (*DelegatedAuthNoop)(nil)

// AddInstance does nothing in the noop implementation
// AddInstance is a no-op. It returns a provider that never has a credential, so a consumer that
// buffers on "no credential yet" would never ship. That is intentional: this impl is wired in
// where delegated auth is compiled out, and nothing registers DELA destinations there.
func (r *DelegatedAuthNoop) AddInstance(_ context.Context, _ delegatedauth.InstanceParams) (delegatedauth.Provider, error) {
	return noopProvider{}, nil
}

// ProvidersFor is a no-op and never has providers to return.
func (r *DelegatedAuthNoop) ProvidersFor(_, _ string) []delegatedauth.Provider {
	return nil
}

// ProviderForDirective implements delegatedauth.Component and never has a credential.
func (r *DelegatedAuthNoop) ProviderForDirective(_, _, _ string) delegatedauth.Provider {
	return nil
}

// RefreshFor is a no-op and never finds a provider.
func (r *DelegatedAuthNoop) RefreshFor(_, _, _ string) bool { return false }

// noopProvider never holds a credential.
type noopProvider struct{}

// Authorize implements delegatedauth.Provider and never authorizes.
func (noopProvider) Authorize(_ http.Header) bool { return false }

// Refresh implements delegatedauth.Provider. No background refresh in the noop impl.
func (noopProvider) Refresh() bool { return false }
