// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package check contains the code to parse the check command output
package check

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// Root is the root object of the check command output
type Root struct {
	Aggregator Aggregator `json:"aggregator"`
}

// Aggregator contains the metrics emitted by a check
type Aggregator struct {
	Metrics []Metric `json:"metrics"`
}

// Metric represents a metric emitted by a check
type Metric struct {
	Host           string      `json:"host"`
	Interval       int         `json:"interval"`
	Metric         string      `json:"metric"`
	Points         [][]float64 `json:"points"`
	SourceTypeName string      `json:"source_type_name"`
	Tags           []string    `json:"tags"`
	Type           string      `json:"type"`
}

// ParseJSONOutput parses the check command json output
func ParseJSONOutput(t *testing.T, check []byte) []Root {
	// On Windows a warning is printed when running the check command with the wrong user
	// This warning is not part of the JSON output and needs to be ignored when parsing
	startIdx := bytes.IndexAny(check, "[{")
	require.NotEqual(t, -1, startIdx, "Failed to find start of JSON output in check output: %v", string(check))

	check = check[startIdx:]

	var data []Root
	err := json.Unmarshal([]byte(check), &data)
	require.NoErrorf(t, err, "Failed to unmarshal check output: %v", string(check))

	return data
}
