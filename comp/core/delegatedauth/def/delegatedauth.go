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
	"github.com/DataDog/datadog-agent/comp/core/delegatedauth/common"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// team: agent-shared-components

// InitParams holds parameters for one-time initialization.
type InitParams struct {
	// Config is used to read settings and write API keys.
	Config pkgconfigmodel.ReaderWriter

	// ProviderConfig contains provider-specific configuration.
	// Use cloudauth.AWSProviderConfig for AWS, etc.
	// If nil, auto-detects from the environment.
	ProviderConfig common.ProviderConfig
}

// InstanceParams configures a single API key instance.
type InstanceParams struct {
	// OrgUUID is the Datadog organization UUID. Required.
	OrgUUID string

	// RefreshInterval in minutes. Defaults to 60 if not specified.
	RefreshInterval int

	// APIKeyConfigKey is where to write the API key (e.g., "api_key", "logs_config.api_key").
	// Defaults to "api_key" if empty.
	APIKeyConfigKey string
}

// Component manages cloud-based delegated authentication.
//
// Usage: Call Initialize() once, then AddInstance() for each API key to manage.
// Each instance starts a background goroutine that periodically refreshes the API key
// and writes it to the config. Thread-safe.
type Component interface {
	// Initialize detects the cloud provider and prepares the component.
	// Must be called once before AddInstance().
	// Returns an error if already initialized or cloud provider detection fails.
	Initialize(params InitParams) error

	// AddInstance configures a specific API key instance.
	// Fetches the initial API key, writes it to config, and starts a background refresh goroutine.
	// Can be called multiple times with different APIKeyConfigKey values.
	// Returns an error if Initialize() was not called or if OrgUUID is empty.
	AddInstance(params InstanceParams) error
}
