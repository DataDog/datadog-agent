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

// team: core-authn

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
	// Requires AdditionalEndpointDirective to also be set. Mutually exclusive with
	// AdditionalEndpointDomain.
	AdditionalEndpointsListConfigKey string

	// AdditionalEndpointDirective is the literal DELA(...) directive text that requested this
	// instance - either a value inside AdditionalEndpointsConfigKey[AdditionalEndpointDomain], or
	// an api_key field inside AdditionalEndpointsListConfigKey. It is replaced in place with the
	// real API key once fetched, and only used when AdditionalEndpointDomain or
	// AdditionalEndpointsListConfigKey is set.
	AdditionalEndpointDirective string

	// FallbackAPIKey, if set, is written in place of a real delegated-auth key when one cannot be
	// obtained: either because no supported cloud provider was detected, or because the initial
	// synchronous fetch fails. This lets dual-shipping keep working (with a static key) while WIF
	// is unavailable or still coming up, instead of shipping nothing at all. Once a real key is
	// successfully fetched it replaces the fallback; a later transient refresh failure does not
	// revert back to the fallback. Parsed from the `fallback=<api_key>` param on a DELA(...)
	// directive - not used for the primary (non-additional-endpoints) delegated auth path, where
	// an existing static APIKeyConfigKey value already survives a failed fetch untouched.
	FallbackAPIKey string

	// ProviderConfig contains provider-specific configuration.
	// Use cloudauth.AWSProviderConfig for AWS, etc.
	// If nil, auto-detects from the environment (only used on first call).
	ProviderConfig common.ProviderConfig
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
}
