// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package autoconnections provides functionality for automatically creating
// PAR connections after successful auto-enrollment.
package autoconnections

// ConnectionDefinition defines all metadata needed to create a connection
type ConnectionDefinition struct {
	// BundleID is the bundle identifier used in allowlist matching
	// Example: "com.datadoghq.http"
	BundleID string

	// IntegrationType is the integration type sent to the API
	// Example: "HTTP", "Kubernetes", "Script"
	IntegrationType string

	Credentials CredentialConfig
}

type CredentialConfig struct {
	// Example: "HTTPNoAuth", "KubernetesServiceAccount", "Script"
	Type string

	// AdditionalFields contains any extra fields needed in the credentials object
	// Example: {"configFileLocation": "/path/to/file"}
	AdditionalFields map[string]interface{}
}
