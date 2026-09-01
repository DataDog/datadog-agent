// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package delegatedauthimpl provides a no-op implementation of the delegatedauth component
package delegatedauthimpl

import (
	"context"
	"io"
	"net/http"

	delegatedauth "github.com/DataDog/datadog-agent/comp/core/delegatedauth/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
)

// Provides list the provided interfaces from the delegatedauth Component
type Provides struct {
	Comp           delegatedauth.Component
	StatusProvider status.InformationProvider
}

type DelegatedAuthNoop struct{}

var _ delegatedauth.Component = (*DelegatedAuthNoop)(nil)

// NewComponent returns a no-op implementation for the delegated auth component
func NewComponent() Provides {
	noop := &DelegatedAuthNoop{}
	// Note: importing log package would require adding it as a dependency, so skipping debug here
	return Provides{
		Comp:           noop,
		StatusProvider: status.NewInformationProvider(noop),
	}
}

// AddInstance is a no-op. It returns a provider that never has a credential, so a consumer that
// buffers on "no credential yet" would never ship. That is intentional: this impl is wired in
// where delegated auth is compiled out, and nothing registers DELA destinations there.
func (d *DelegatedAuthNoop) AddInstance(_ context.Context, _ delegatedauth.InstanceParams) (delegatedauth.Provider, error) {
	return noopProvider{}, nil
}

// ProvidersFor is a no-op and never has providers to return.
func (d *DelegatedAuthNoop) ProvidersFor(_, _ string) []delegatedauth.Provider {
	return nil
}

// ProviderForDirective implements delegatedauth.Component and never has a credential.
func (d *DelegatedAuthNoop) ProviderForDirective(_, _, _ string) delegatedauth.Provider {
	return nil
}

// RefreshFor is a no-op and never finds a provider.
func (d *DelegatedAuthNoop) RefreshFor(_, _, _ string) bool { return false }

// noopProvider never holds a credential.
type noopProvider struct{}

// Authorize implements delegatedauth.Provider and never authorizes.
func (noopProvider) Authorize(_ http.Header) bool { return false }

// Refresh implements delegatedauth.Provider. No background refresh in the noop impl.
func (noopProvider) Refresh() bool { return false }

// Status Provider implementation for noop

// Name returns the name for status sorting
func (d *DelegatedAuthNoop) Name() string {
	return "Delegated Auth"
}

// Section returns the section name for status grouping
func (d *DelegatedAuthNoop) Section() string {
	return "delegatedauth"
}

// JSON populates the status stats map
func (d *DelegatedAuthNoop) JSON(_ bool, stats map[string]interface{}) error {
	stats["enabled"] = false
	return nil
}

// Text renders the text status output
func (d *DelegatedAuthNoop) Text(_ bool, buffer io.Writer) error {
	_, err := buffer.Write([]byte("Delegated Authentication is not enabled\n"))
	return err
}

// HTML renders the HTML status output
func (d *DelegatedAuthNoop) HTML(_ bool, buffer io.Writer) error {
	_, err := buffer.Write([]byte("<div class=\"stat\"><span class=\"stat_title\">Delegated Authentication</span><span class=\"stat_data\">Not enabled</span></div>\n"))
	return err
}
