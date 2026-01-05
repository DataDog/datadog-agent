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

// team: agent-shared-components

// ConfigParams holds parameters for delegated auth configuration.
//
// These parameters are typically read from the agent's configuration file and passed to the
// Configure method during agent startup.
type ConfigParams struct {
	// Config is the config component used to read settings and write the API key.
	// This must be provided as a config.Component, but is declared as interface{} to avoid import cycles.
	// The implementation will type-assert to config.Component.
	Config interface{}

	// Enabled indicates whether delegated authentication should be active.
	// When false, the component does nothing.
	Enabled bool

	// Provider specifies the cloud provider to use for authentication.
	// Currently supported values: "aws"
	Provider string

	// OrgUUID is the Datadog organization UUID for which to fetch credentials.
	// This is required when Enabled is true.
	OrgUUID string

	// RefreshInterval specifies how often to refresh the API key, in minutes.
	// Default is 60 minutes if not specified or set to 0.
	RefreshInterval int

	// AWSRegion is the AWS region to use for AWS provider authentication.
	// Only used when Provider is "aws".
	AWSRegion string
}

// Component manages delegated authentication with cloud providers.
//
// Thread Safety:
//   - All methods are safe to call concurrently from multiple goroutines.
//   - Configure should be called once during agent startup before other components initialize.
//   - GetAPIKey and RefreshAPIKey can be called at any time after Configure.
//
// Lifecycle:
//   - The component is created with minimal dependencies during fx initialization.
//   - Configure() must be called explicitly after config is loaded to activate the component.
//   - If Configure is not called or Enabled is false, all methods are no-ops.
//   - When configured, the component starts a background goroutine for periodic refreshes.
//
// API Key Management:
//   - When a new API key is fetched, it's automatically written to config via config.Set("api_key", ...).
//   - This triggers the config notification system, allowing other components to react to API key changes.
//   - The component maintains an internal cache of the current API key.
//   - Refresh operations use exponential backoff on failures (doubling interval, capped at 1 hour).
type Component interface {
	// Configure initializes the delegated auth component with the provided configuration parameters.
	//
	// This method should be called once during agent startup, after the config is loaded but before
	// other components that depend on the API key are initialized. Calling Configure multiple times
	// is not supported and may cause undefined behavior.
	//
	// If params.Enabled is false, this method returns immediately and the component remains inactive.
	//
	// When params.Enabled is true, Configure will:
	//   1. Validate the configuration parameters (OrgUUID, Provider, etc.)
	//   2. Fetch the initial API key from the cloud provider
	//   3. Write the API key to config via config.Set("api_key", ...)
	//   4. Start a background goroutine for periodic API key refreshes
	//
	// The method does not block on the initial API key fetch. If the fetch fails, the error is logged
	// and exponential backoff is used for retries in the background goroutine.
	//
	// Goroutines:
	//   - If enabled, Configure starts exactly one background goroutine for periodic refreshes.
	//   - The goroutine continues until the agent shuts down.
	//   - There is no explicit Stop or Close method; cleanup happens automatically on process exit.
	Configure(config ConfigParams)
}
