// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package delegatedauth manages cloud-based delegated authentication for the agent.
//
// It fetches and refreshes Datadog API keys from cloud providers (e.g., AWS IAM) and
// automatically updates the agent's configuration.
package delegatedauth

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// team: credential-management

// InstanceParams configures a single API key instance.
type InstanceParams struct {
	// Config is used to read settings and write API keys. Required.
	// IMPORTANT: Only the Config from the FIRST AddInstance call is used.
	// Subsequent calls must pass the same config instance; passing a different
	// config will be ignored and a warning will be logged.
	Config pkgconfigmodel.ReaderWriter

	// OrgUUID is the Datadog organization UUID. Required.
	OrgUUID string

	// RefreshInterval in minutes. Defaults to 60 if not specified.
	RefreshInterval int

	// APIKeyConfigKey is where to write the API key (e.g., "api_key", "logs_config.api_key").
	// Required, even when AdditionalEndpointDomain is set: it is used as an internal
	// bookkeeping/status-display key in that mode (e.g. "additional_endpoints[<domain>]"),
	// since the API key itself is not written to this config key in that case.
	APIKeyConfigKey string

	// AdditionalEndpointDomain, if set, causes the fetched API key to be merged into the
	// map-shape config value at AdditionalEndpointsConfigKey under this domain (replacing the
	// DELA(...) directive that requested it) instead of being written to APIKeyConfigKey as a
	// flat value. This supports dual/multi-org shipping via map-shape `additional_endpoints`-style
	// config (e.g. the top-level `additional_endpoints`, `apm_config.additional_endpoints`, ...).
	// Requires AdditionalEndpointsConfigKey and AdditionalEndpointDirective to also be set.
	// Mutually exclusive with AdditionalEndpointsListConfigKey.
	AdditionalEndpointDomain string

	// AdditionalEndpointsConfigKey is the config path of the map-shape `additional_endpoints`-style
	// value (domain -> list of API keys) that AdditionalEndpointDomain refers into, e.g.
	// "additional_endpoints" or "apm_config.additional_endpoints". Required when
	// AdditionalEndpointDomain is set.
	AdditionalEndpointsConfigKey string

	// AdditionalEndpointsListConfigKey, if set, causes the fetched API key to be merged into the
	// list-shape config value at this path (a list of {api_key, Host, Port, ...} entries, e.g.
	// "logs_config.additional_endpoints", "database_monitoring.samples.additional_endpoints"),
	// replacing the entry whose api_key still holds the DELA(...) directive that requested it.
	// Requires AdditionalEndpointDirective and ListEntryIndex to also be set. Mutually exclusive
	// with AdditionalEndpointDomain.
	AdditionalEndpointsListConfigKey string

	// ListEntryIndex is this entry's position within the list-shape value at
	// AdditionalEndpointsListConfigKey. Only used (and required) when
	// AdditionalEndpointsListConfigKey is set - it's the identity Component.IsManaged uses to
	// find this instance again, since a list can hold several DELA(...) entries at the same key.
	ListEntryIndex int

	// AdditionalEndpointDirective is the literal DELA(...) directive text that requested this
	// instance - either a value inside AdditionalEndpointsConfigKey[AdditionalEndpointDomain], or
	// an api_key field inside AdditionalEndpointsListConfigKey. It is replaced in place with the
	// real API key once fetched, and only used when AdditionalEndpointDomain or
	// AdditionalEndpointsListConfigKey is set.
	AdditionalEndpointDirective string

	// TargetSite is the Datadog site to exchange the auth proof against, e.g. a list-shape
	// additional_endpoints entry's Host field. Falls back to AdditionalEndpointDomain when unset
	// (the map-shape case, where the domain key itself is the target site), and to the agent's
	// primary site when both are unset (the flat, non-additional-endpoints case).
	TargetSite string

	// FallbackAPIKey, if set, is written in place of a real delegated-auth key when one cannot be
	// obtained (no supported cloud provider, or the initial synchronous fetch fails), so
	// dual-shipping ships with a static key instead of nothing. A later successful fetch replaces
	// it and a later transient failure does not revert back to it. Parsed from a DELA(...)
	// directive's `fallback=<api_key>` param.
	FallbackAPIKey string

	// ProviderConfig contains provider-specific configuration.
	// Use cloudauth.AWSProviderConfig for AWS, etc.
	// If nil, auto-detects from the environment (only used on first call).
	ProviderConfig common.ProviderConfig
}

// Target identifies which delegated-auth instance a Component.IsManaged query is asking about.
// It mirrors the identity-establishing subset of InstanceParams' routing fields - set exactly one
// of the three combinations:
//   - APIKeyConfigKey alone, for a flat (non-additional-endpoints) instance, e.g. "api_key" or
//     "logs_config.api_key".
//   - AdditionalEndpointsConfigKey + AdditionalEndpointDomain, for a map-shape instance.
//   - AdditionalEndpointsListConfigKey + ListEntryIndex, for a list-shape instance.
type Target struct {
	APIKeyConfigKey string

	AdditionalEndpointsConfigKey string
	AdditionalEndpointDomain     string

	AdditionalEndpointsListConfigKey string
	ListEntryIndex                   int
}

// Component manages cloud-based delegated authentication.
//
// Usage: Call AddInstance() for each API key to manage.
// The first call auto-detects the cloud provider and initializes the component.
// Each instance starts a background goroutine that periodically refreshes the API key
// and writes it to the config. Thread-safe.
type Component interface {
	// AddInstance configures a specific API key instance.
	// On the first call, it detects the cloud provider and initializes the component.
	// Fetches the initial API key, writes it to config, and starts a background refresh goroutine.
	// Can be called multiple times with different APIKeyConfigKey values.
	// The context is used for the initial API key fetch and cloud provider detection;
	// background refresh goroutines use their own cancellable context.
	// Returns an error if Config or OrgUUID is empty.
	AddInstance(ctx context.Context, params InstanceParams) error

	// Refresh nudges instances to retry sooner than their normal backoff interval, throttled
	// per-instance to avoid hammering the auth-proof exchange. Mirrors
	// comp/core/secrets.Component.Refresh()'s contract. Non-blocking - never fetches inline.
	// Returns false only when there are no delegated-auth instances at all.
	Refresh() bool

	// IsManaged reports whether an active instance currently manages target. Unlike string-sniffing
	// the api_key config value for a DELA(...) directive, this stays true even after the directive
	// has already been resolved to a real key. Callers should use this to decide whether a 403 on
	// an endpoint is a transient WIF auth failure rather than a bad static key.
	IsManaged(target Target) bool
}
