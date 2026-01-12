// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets decodes secret values by invoking the configured executable command
package secrets

// team: agent-configuration

// ConfigParams holds parameters for configuration
type ConfigParams struct {
	Type                         string
	Config                       map[string]interface{}
	Command                      string
	Arguments                    []string
	Timeout                      int
	MaxSize                      int
	RefreshInterval              int
	RefreshIntervalScatter       bool
	GroupExecPerm                bool
	RemoveLinebreak              bool
	RunPath                      string
	AuditFileMaxSize             int
	ScopeIntegrationToNamespace  bool
	AllowedNamespace             []string
	ImageToHandle                map[string][]string
	APIKeyFailureRefreshInterval int
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
	// Refresh will resolve secret handles again, notifying any subscribers of changed values.
	// If updateNow is true, the function performs the refresh immediately and blocks, returning an informative message suitable for user display.
	// If updateNow is false, the function will asynchronously perform a refresh, and may fail to refresh due to throttling. No message is returned, just an empty string.
	Refresh(updateNow bool) (string, error)
	// RemoveOrigin removes a origin from the internal cache of the secret component. This does not remove secrets
	// from the cache but the reference where those secrets are used.
	RemoveOrigin(origin string)
}
