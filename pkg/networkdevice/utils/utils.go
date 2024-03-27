// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package utils

import "github.com/DataDog/datadog-agent/pkg/version"

// CopyStrings makes a copy of a list of strings
func CopyStrings(tags []string) []string {
	newTags := make([]string, len(tags))
	copy(newTags, tags)
	return newTags
}

// GetAgentVersionTag returns agent version tag
func GetAgentVersionTag() string {
	return "agent_version:" + version.AgentVersion
}

// BoolToFloat64 converts a true/false boolean into a 1.0 or 0.0 float
func BoolToFloat64(val bool) float64 {
	if val {
		return 1.
	}
	return 0.
}
