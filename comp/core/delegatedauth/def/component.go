// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package delegatedauth manages cloud-based delegated authentication for the agent.
// It fetches and refreshes Datadog API keys from cloud providers and writes them
// to the agent's configuration.
package delegatedauth

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/credential"
)

// team: credential-management delegated-auth-login

// Provider is an alias for credential.Provider, the canonical interface declared in
// pkg/credential. The alias keeps existing imports of delegatedauth.Provider working
// while making pkg/credential the single source of truth that both comp/ and pkg/trace
// can depend on.
type Provider = credential.Provider

// InstanceParams configures a single API key instance.
//
// There are two usage shapes, distinguished by SkipConfigWriteback:
//
//   - Flat-key (write-back): the resolved key is written into the config slot named by
//     APIKeyConfigKey, and consumers pick it up via config OnUpdate callbacks. This is
//     the original delivery path used by the delegated_auth.* config sections.
//
//   - Directive (provider): the key is delivered through a Provider returned by
//     AddInstance, and nothing is written to config. SkipConfigWriteback must be true.
//     ConfigKey + Destination identify the provider so consumers can find it via
//     Component.ProvidersFor, and Directive disambiguates multiple orgs on one host.
//
// Fields marked [flat] are only used when SkipConfigWriteback is false.
// Fields marked [directive] are only used when SkipConfigWriteback is true.
// Fields marked [shared] are used in both paths.
type InstanceParams struct {
	// [shared] Config is used to read settings and write API keys. Only the Config from
	// the first AddInstance call is used; later calls must pass the same instance.
	Config pkgconfigmodel.ReaderWriter

	// [shared] OrgUUID is the Datadog organization UUID. Required.
	OrgUUID string

	// [shared] RefreshInterval in minutes. Defaults to 60 if not specified.
	RefreshInterval int

	// [shared] APIKeyConfigKey is a unique identifier for this instance. In flat-key mode it is
	// also where the resolved key is written (e.g. "api_key", "logs_config.api_key"). In
	// directive mode it is bookkeeping only — the key is delivered via Provider. Required.
	APIKeyConfigKey string

	// [flat] AdditionalEndpointDomain, if set, routes the fetched key into the map-shape
	// config at AdditionalEndpointsConfigKey under this domain, replacing the
	// DELA(...) directive. Mutually exclusive with AdditionalEndpointsListConfigKey.
	// Requires AdditionalEndpointsConfigKey and AdditionalEndpointDirective.
	AdditionalEndpointDomain string

	// [flat] AdditionalEndpointsConfigKey is the config path of the map-shape
	// additional_endpoints value (domain -> []keys). Required when
	// AdditionalEndpointDomain is set.
	AdditionalEndpointsConfigKey string

	// [flat] AdditionalEndpointKeyIndex is this entry's position in the domain's key list at
	// AdditionalEndpointsConfigKey. Optional, but recommended when set alongside
	// AdditionalEndpointDomain: it disambiguates this instance's own entry from another entry
	// under the same domain that happens to share the same value (e.g. a fallback API key that
	// matches a different, unrelated static key). Falls back to a value-only scan if the index
	// doesn't point at a matching entry (e.g. the list was reordered).
	AdditionalEndpointKeyIndex int

	// [flat] AdditionalEndpointsListConfigKey, if set, routes the fetched key into the
	// list-shape config at this path, replacing the entry whose api_key holds the
	// DELA(...) directive. Mutually exclusive with AdditionalEndpointDomain.
	// Requires AdditionalEndpointDirective and ListEntryIndex.
	AdditionalEndpointsListConfigKey string

	// [flat] ListEntryIndex is this entry's position in the list at
	// AdditionalEndpointsListConfigKey. Required when that field is set.
	ListEntryIndex int

	// [flat] AdditionalEndpointDirective is the literal DELA(...) directive text to
	// replace with the real key once fetched. Used only when
	// AdditionalEndpointDomain or AdditionalEndpointsListConfigKey is set.
	AdditionalEndpointDirective string

	// [flat] WritebackPath addresses the exact config value to replace. For example,
	// {"additional_endpoints", domain, "1"} selects one key in a map-shape endpoint.
	// When set, it takes precedence over the legacy map/list write-back fields above.
	WritebackPath []string

	// [shared] TargetSite is the Datadog site to exchange the auth proof against.
	// Falls back to AdditionalEndpointDomain, then to the agent's primary site.
	TargetSite string

	// [shared] FallbackAPIKey, if set, is written when no delegated-auth key can be
	// obtained so dual-shipping still works. A later successful fetch replaces it.
	// Parsed from a DELA(...) directive's fallback=<api_key> param.
	FallbackAPIKey string

	// [shared] ProviderConfig contains provider-specific configuration.
	// If nil, auto-detects from the environment (only used on first call).
	ProviderConfig common.ProviderConfig

	// [directive] SkipConfigWriteback stops the component writing the resolved key back into
	// the config slot named by APIKeyConfigKey / AdditionalEndpoint*. Set it when the consumer
	// takes the credential from the returned Provider instead, which keeps the key out of the
	// config tree entirely.
	//
	// Defaults to false so existing callers keep the write-back delivery path. Both at once would
	// double-deliver: the consumer would see a provider-backed credential AND a static key
	// appearing in config for the same destination.
	SkipConfigWriteback bool

	// [directive] Directive is the raw DELA(...) text this instance was created from. It identifies the
	// instance among several sharing one destination, which Destination alone cannot do: two
	// orgs may legitimately dual-ship to the same domain.
	Directive string

	// [directive] ConfigKey is the setting the credential belongs to (e.g. "additional_endpoints"), forming
	// the other half of the Component.ProvidersFor lookup. Leave it empty for a flat key, where
	// APIKeyConfigKey already identifies the slot.
	ConfigKey string

	// [directive] Destination identifies what this credential is for, so consumers can look up the provider
	// they need via Component.ProvidersFor. For an additional endpoint it is the domain or host;
	// for a flat key it may be left empty.
	Destination string
}

// ProviderKey is the (configKey, destination) pair this instance's Provider is registered under,
// and the pair a consumer must pass to Component.ProvidersFor to find it. Both the component and
// its mock derive the key here so they cannot disagree about where a provider lives.
//
// In directive mode the caller sets ConfigKey (the setting path, e.g. "additional_endpoints").
// In flat-key mode ConfigKey is empty, so we fall back through the write-back fields and
// ultimately to APIKeyConfigKey, which is always set.
func (p InstanceParams) ProviderKey() (configKey, destination string) {
	configKey = p.ConfigKey
	if configKey == "" {
		configKey = p.AdditionalEndpointsConfigKey
	}
	if configKey == "" {
		configKey = p.AdditionalEndpointsListConfigKey
	}
	if configKey == "" {
		configKey = p.APIKeyConfigKey
	}
	return configKey, p.Destination
}

// Component manages cloud-based delegated authentication.
// Call AddInstance for each API key to manage; the first call initializes the
// component and each instance starts a background refresh goroutine. Thread-safe.
type Component interface {
	// AddInstance configures a specific API key instance.
	// The context is used for the initial fetch and provider detection;
	// background refresh uses its own cancellable context.
	// Returns an error if Config or OrgUUID is empty.
	//
	// The returned Provider tracks this instance's credential for the life of the process. It is
	// non-nil whenever the error is nil, and reports "no credential yet" until the first exchange
	// succeeds (or fails and a FallbackAPIKey is configured).
	AddInstance(ctx context.Context, params InstanceParams) (Provider, error)

	// ProvidersFor returns the providers registered for a config setting and destination, in the
	// order their directives were discovered. Consumers that are built after discovery has run -
	// the forwarder, for one - use this instead of holding on to what AddInstance returned.
	//
	// A destination can legitimately have more than one provider (one per org), which is why this
	// returns a slice rather than a single provider keyed by name.
	ProvidersFor(configKey, destination string) []Provider

	// ProviderForDirective returns the provider registered for one specific DELA(...) directive,
	// for consumers that create one destination per directive and so must not mix them up.
	// Returns nil when no instance was registered for that directive, e.g. because it named an
	// unsupported provider - the caller must then refuse to send rather than send unauthenticated.
	ProviderForDirective(configKey, destination, directive string) Provider

	// RefreshFor requests an immediate refresh for the provider currently serving credential at
	// configKey and destination. It returns false when no matching provider can be refreshed.
	RefreshFor(configKey, destination, credential string) bool
}
