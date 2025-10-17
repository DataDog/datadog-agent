// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package secrets decodes secret values by invoking the configured executable command
package secrets

// team: agent-configuration

// ConfigParams holds parameters for configuration
type ConfigParams struct {
	Type                        string
	Config                      map[string]interface{}
	Command                     string
	Arguments                   []string
	Timeout                     int
	MaxSize                     int
	RefreshInterval             int
	RefreshIntervalScatter      bool
	GroupExecPerm               bool
	RemoveLinebreak             bool
	RunPath                     string
	AuditFileMaxSize            int
	ScopeIntegrationToNamespace bool
	AllowedNamespace            []string
	ImageToHandle               map[string][]string
}

// Component is the component type.
type Component interface {
	// Configure the executable command that is used for decoding secrets
	Configure(config ConfigParams)
	// Resolve resolves the secrets in the given yaml data by replacing secrets handles by their corresponding secret value
	Resolve(data []byte, origin string, imageName string, kubeNamespace string) ([]byte, error)
	// SubscribeToChanges registers a callback to be invoked whenever secrets are resolved or refreshed
	SubscribeToChanges(callback SecretChangeCallback)
	// Refresh will resolve secret handles again, notifying any subscribers of changed values
	Refresh() (string, error)
}
