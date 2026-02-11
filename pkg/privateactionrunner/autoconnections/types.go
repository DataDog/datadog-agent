// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

// ConnectionDefinition defines all metadata needed to create a connection
type ConnectionDefinition struct {
	// FQNPrefix is the identifier used in allowlist matching
	// Example: "com.datadoghq.script"
	FQNPrefix string

	// IntegrationType is the integration type sent to the API
	// Example: "Kubernetes", "Script"
	IntegrationType string

	Credentials CredentialConfig
}

type CredentialConfig struct {
	// Example: "KubernetesServiceAccount", "Script"
	Type string

	// AdditionalFields contains any extra fields needed in the credentials object
	// Example: {"configFileLocation": "/path/to/file"}
	AdditionalFields map[string]interface{}
}
