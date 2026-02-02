// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package autoconnections

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// SupportedConnections defines all connection types that can be auto-created
var SupportedConnections = map[string]ConnectionDefinition{
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
				"configFileLocation": "/etc/dd-action-runner/config/credentials/script.yaml",
			},
		},
	},
}

func GetConnectionDefinition(key string) (ConnectionDefinition, bool) {
	def, ok := SupportedConnections[key]
	return def, ok
}

func GetBundleKeys() []string {
	keys := make([]string, 0, len(SupportedConnections))
	for key := range SupportedConnections {
		keys = append(keys, key)
	}
	return keys
}

func matchesPattern(pattern, bundleID string) bool {
	if pattern == bundleID {
		return true
	}

	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		if strings.HasPrefix(bundleID, prefix) {
			return true
		}
	}

	if strings.HasPrefix(pattern, bundleID+".") {
		return true
	}

	if pattern == "com.datadoghq.*" {
		return strings.HasPrefix(bundleID, "com.datadoghq.")
	}

	return false
}

func allowlistContainsBundle(allowlist []string, bundleID string) bool {
	for _, pattern := range allowlist {
		if matchesPattern(pattern, bundleID) {
			return true
		}
	}
	return false
}

func DetermineConnectionsToCreate(allowlist []string) []ConnectionDefinition {
	if len(allowlist) == 0 {
		return []ConnectionDefinition{}
	}

	result := []ConnectionDefinition{}

	for _, definition := range SupportedConnections {
		if allowlistContainsBundle(allowlist, definition.BundleID) {
			result = append(result, definition)
		}
	}

	return result
}

func GenerateConnectionName(definition ConnectionDefinition, runnerID string) string {
	return fmt.Sprintf("%s (%s)", definition.IntegrationType, runnerID)
}

func extractRunnerIDFromURN(urn string) (string, error) {
	parts, err := util.ParseRunnerURN(urn)

	if err != nil {
		return "", err
	}

	return parts.RunnerID, nil
}
