// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package delegatedauth provides a component for managing delegated authentication with cloud providers.
//
// This component handles fetching and refreshing API keys from cloud provider authentication systems
// (such as AWS IAM), and automatically updates the agent's configuration with the retrieved API key.
// The component runs a background goroutine to periodically refresh the API key with exponential backoff on failures.
package delegatedauth

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// team: agent-shared-components

// InitParams holds parameters for initializing the delegated auth component.
//
// These parameters are used once during initialization to resolve the cloud provider.
type InitParams struct {
	// Config is the config component used to read settings and write API keys.
	Config pkgconfigmodel.ReaderWriter

	// Provider specifies the cloud provider to use for authentication.
	// Optional. Currently supported: "aws" (AWS via EC2 metadata service).
	// If empty, the component will auto-detect the cloud provider from the environment.
	// For testing, this can be explicitly set to bypass auto-detection.
	Provider string

	// AWSRegion is the AWS region to use for AWS provider authentication.
	// Optional. If not specified, the component will auto-detect it from the EC2 metadata service.
	// For testing, this can be explicitly set to bypass auto-detection.
	AWSRegion string
}

// InstanceParams holds parameters for configuring a delegated auth instance.
//
// Each instance represents one API key that should be fetched and refreshed.
type InstanceParams struct {
	// OrgUUID is the Datadog organization UUID for which to fetch credentials.
	// This is required and must be unique per instance.
	OrgUUID string

	// RefreshInterval specifies how often to refresh the API key, in minutes.
	// Default is 60 minutes if not specified or set to 0.
	RefreshInterval int

	// APIKeyConfigKey specifies the configuration key where the fetched API key should be written.
	// Examples: "api_key" (default), "logs_config.api_key"
	// This must be unique per instance. If empty, defaults to "api_key".
	APIKeyConfigKey string
}

// Component manages delegated authentication with cloud providers.
//
// Thread Safety:
//   - All methods are safe to call concurrently from multiple goroutines.
//   - AddInstance can be called multiple times with different APIKeyConfigKey values to manage
//     separate API keys for different products (e.g., global api_key and logs_config.api_key).
//
// Lifecycle:
//   - The component is created with minimal dependencies during fx initialization.
//   - Initialize() must be called once before AddInstance() to resolve the cloud provider.
//   - AddInstance() can then be called multiple times to configure different API keys.
//   - If neither method is called, the component does nothing.
//   - When instances are added, the component starts background goroutines for periodic refreshes.
//   - Each AddInstance() call with a unique APIKeyConfigKey creates an independent refresh loop.
//
// API Key Management:
//   - When a new API key is fetched, it's automatically written to config via config.Set(APIKeyConfigKey, ...).
//   - This triggers the config notification system, allowing other components to react to API key changes.
//   - The component maintains internal caches of API keys per config key.
//   - Refresh operations use exponential backoff on failures (doubling interval, capped at 1 hour).
type Component interface {
	// Initialize resolves the cloud provider and prepares the component for use.
	//
	// This method must be called once before AddInstance(). It performs the expensive operation
	// of detecting the cloud provider and obtaining cloud-specific configuration (e.g., AWS region).
	//
	// Initialize will:
	//   1. Auto-detect the cloud provider (or use the provided override)
	//   2. Obtain cloud-specific configuration (e.g., AWS region, credentials)
	//   3. Cache the resolved provider for use by all instances
	//
	// Returns an error if:
	//   - Already initialized (calling Initialize twice is an error)
	//   - Cloud provider detection fails
	//   - Cloud provider is unsupported
	//
	// Thread safety: Safe to call concurrently, but will return an error if already initialized.
	Initialize(params InitParams) error

	// AddInstance configures delegated auth for a specific API key.
	//
	// This method can be called multiple times with different APIKeyConfigKey values to manage
	// separate API keys. For example:
	//   - AddInstance(params with APIKeyConfigKey="api_key") for global API key
	//   - AddInstance(params with APIKeyConfigKey="logs_config.api_key") for logs-specific API key
	//
	// Each call with a unique APIKeyConfigKey creates an independent refresh loop.
	// Calling AddInstance multiple times with the same APIKeyConfigKey will replace the previous configuration.
	//
	// AddInstance will:
	//   1. Validate the configuration parameters (OrgUUID, APIKeyConfigKey, etc.)
	//   2. Fetch the initial API key from the cloud provider
	//   3. Write the API key to config via config.Set(params.APIKeyConfigKey, ...)
	//   4. Start a background goroutine for periodic API key refreshes
	//
	// The method does not block on the initial API key fetch. If the fetch fails, the error is logged
	// and exponential backoff is used for retries in the background goroutine.
	//
	// Returns an error if:
	//   - Initialize has not been called yet
	//   - OrgUUID is empty
	//   - APIKeyConfigKey is empty after defaulting
	//
	// Goroutines:
	//   - Each AddInstance call with a unique APIKeyConfigKey starts a background goroutine for periodic refreshes.
	//   - Goroutines continue until the agent shuts down.
	//   - There is no explicit Stop or Close method; cleanup happens automatically on process exit.
	//
	// Thread safety: Safe to call concurrently after Initialize() has been called.
	AddInstance(params InstanceParams) error
}
