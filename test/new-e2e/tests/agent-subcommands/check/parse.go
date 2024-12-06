// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

type root struct {
	Aggregator aggregator `json:"aggregator"`
}

type aggregator struct {
	Metrics []metric `json:"metrics"`
}

type metric struct {
	Host           string   `json:"host"`
	Interval       int      `json:"interval"`
	Metric         string   `json:"metric"`
	Points         [][]int  `json:"points"`
	SourceTypeName string   `json:"source_type_name"`
	Tags           []string `json:"tags"`
	Type           string   `json:"type"`
}

func parseCheckOutput(t *testing.T, check []byte) []root {
	// On Windows a warning is printed when running the check command with the wrong user
	// This warning is not part of the JSON output and needs to be ignored when parsing
	startIdx := bytes.IndexAny(check, "[{")
	require.NotEqual(t, -1, startIdx, "Failed to find start of JSON output in check output: %v", string(check))

	check = check[startIdx:]

	var data []root
	err := json.Unmarshal([]byte(check), &data)
	require.NoErrorf(t, err, "Failed to unmarshal check output: %v", string(check))

	return data
}
