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
	// `additional_endpoints` config map under this domain (replacing the DELA(...) directive
	// that requested it) instead of being written to APIKeyConfigKey as a flat value. This
	// supports dual/multi-org shipping via `additional_endpoints`. Requires
	// AdditionalEndpointDirective to also be set.
	AdditionalEndpointDomain string

	// AdditionalEndpointDirective is the literal DELA(...) directive text that appears in
	// `additional_endpoints[AdditionalEndpointDomain]` and requested this instance. It is
	// replaced in place with the real API key once fetched, and only used when
	// AdditionalEndpointDomain is set.
	AdditionalEndpointDirective string

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
