// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/version"
)

// CreateStringBatches batches strings into chunks with specific size
func CreateStringBatches(elements []string, size int) ([][]string, error) {
	var batches [][]string

	if size <= 0 {
		return nil, fmt.Errorf("batch size must be positive. invalid size: %d", size)
	}

	for i := 0; i < len(elements); i += size {
		j := i + size
		if j > len(elements) {
			j = len(elements)
		}
		batch := elements[i:j]
		batches = append(batches, batch)
	}

	return batches, nil
}

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
