// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets decodes secret values by invoking the configured executable command
package secrets

// team: fleet-automation

// SecretBackendConfig holds the configuration for a single named backend in multi_secret_backends.
type SecretBackendConfig struct {
	Type   string                 `mapstructure:"type"`
	Config map[string]interface{} `mapstructure:"config"`
}

// ConfigParams holds parameters for configuration
type ConfigParams struct {
	Config                       map[string]interface{}
	MultiBackends                map[string]SecretBackendConfig
	ImageToHandle                map[string][]string
	Type                         string
	Command                      string
	RunPath                      string
	Arguments                    []string
	AllowedNamespace             []string
	Timeout                      int
	MaxSize                      int
	RefreshInterval              int
	AuditFileMaxSize             int
	APIKeyFailureRefreshInterval int
	RefreshIntervalScatter       bool
	GroupExecPerm                bool
	RemoveLinebreak              bool
	ScopeIntegrationToNamespace  bool
}

// Component is the component type.
type Component interface {
	// Configure the executable command that is used for decoding secrets
	Configure(config ConfigParams)
	// Resolve resolves the secrets in the given yaml data by replacing secrets handles by their corresponding secret value.
	//
	// Setting 'notify' to true will send notifications for any resolve secrets. This is meant for callers that when
	// to replace handle themselves in memory. only the configuration requires this at the moment.
	Resolve(data []byte, origin string, imageName string, kubeNamespace string, notify bool) ([]byte, error)
	// SubscribeToChanges registers a callback to be invoked whenever secrets are resolved or refreshed
	SubscribeToChanges(callback SecretChangeCallback)
	// Refresh schedules a throttled asynchronous secret refresh. Returns true if the
	// secret refresh mechanism is enabled (backend configured and refresh interval set).
	Refresh() bool
	// RefreshNow performs an immediate blocking secret refresh, returning an informative message suitable for user display.
	RefreshNow() (string, error)
	// IsValueFromSecret returns true if the given value was ever resolved from a secret handle.
	IsValueFromSecret(value string) bool
	// RemoveOrigin removes a origin from the internal cache of the secret component. This does not remove secrets
	// from the cache but the reference where those secrets are used.
	RemoveOrigin(origin string)
}
