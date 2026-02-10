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
	"kubernetes": {
		FQNPrefix:       "com.datadoghq.kubernetes",
		IntegrationType: "Kubernetes",
		Credentials: CredentialConfig{
			Type:             "KubernetesServiceAccount",
			AdditionalFields: nil,
		},
	},
	"script": {
		FQNPrefix:       "com.datadoghq.script",
		IntegrationType: "Script",
		Credentials: CredentialConfig{
			Type: "Script",
			AdditionalFields: map[string]interface{}{
				"configFileLocation": getScriptConfigPath(),
			},
		},
	},
}

func actionsAllowlistContainsBundle(actionsAllowlist []string, fqnPrefix string) bool {
	for _, fqn := range actionsAllowlist {
		// The agent actionsAllowlist only supports FQNs
		if strings.HasPrefix(fqn, fqnPrefix) {
			return true
		}
	}
	return false
}

func DetermineConnectionsToCreate(actionsAllowlist []string) []ConnectionDefinition {
	if len(actionsAllowlist) == 0 {
		return []ConnectionDefinition{}
	}

	var result []ConnectionDefinition

	for _, definition := range supportedConnections {
		if actionsAllowlistContainsBundle(actionsAllowlist, definition.FQNPrefix) {
			result = append(result, definition)
		}
	}

	return result
}

func GenerateConnectionName(definition ConnectionDefinition, runnerName string) string {
	return fmt.Sprintf("%s (%s)", definition.IntegrationType, runnerName)
}
