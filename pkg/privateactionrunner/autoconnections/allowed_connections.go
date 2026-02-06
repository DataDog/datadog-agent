// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"fmt"
	"strings"
)

// supportedConnections defines all connection types that can be auto-created
var supportedConnections = map[string]ConnectionDefinition{
	"http": {
		BundleID:        "com.datadoghq.http",
		IntegrationType: "HTTP",
		Credentials: CredentialConfig{
			Type:             "HTTPNoAuth",
			AdditionalFields: nil,
		},
	},
	"kubernetes": {
		BundleID:        "com.datadoghq.kubernetes",
		IntegrationType: "Kubernetes",
		Credentials: CredentialConfig{
			Type:             "KubernetesServiceAccount",
			AdditionalFields: nil,
		},
	},
	"script": {
		BundleID:        "com.datadoghq.script",
		IntegrationType: "Script",
		Credentials: CredentialConfig{
			Type: "Script",
			AdditionalFields: map[string]interface{}{
				"configFileLocation": GetScriptConfigPath(),
			},
		},
	},
}

func bundleAllowlistContainsBundle(bundleAllowlist []string, bundleID string) bool {
	for _, pattern := range bundleAllowlist {
		// The agent bundleAllowlist only supports FQNs
		if strings.HasPrefix(pattern, bundleID) {
			return true
		}
	}
	return false
}

func DetermineConnectionsToCreate(bundleAllowlist []string) []ConnectionDefinition {
	if len(bundleAllowlist) == 0 {
		return []ConnectionDefinition{}
	}

	var result []ConnectionDefinition

	for _, definition := range supportedConnections {
		if bundleAllowlistContainsBundle(bundleAllowlist, definition.BundleID) {
			result = append(result, definition)
		}
	}

	return result
}

func GenerateConnectionName(definition ConnectionDefinition, runnerName string) string {
	return fmt.Sprintf("%s (%s)", definition.IntegrationType, runnerName)
}
