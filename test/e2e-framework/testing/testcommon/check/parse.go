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
	Runner     Runner     `json:"runner"`
}

// Aggregator contains the metrics emitted by a check
type Aggregator struct {
	Metrics       []Metric       `json:"metrics"`
	ServiceChecks []ServiceCheck `json:"service_checks"`
	Events        []Event        `json:"events"`
}

// Runner contains the check execution information
type Runner struct {
	TotalRuns     int `json:"TotalRuns"`
	TotalErrors   int `json:"TotalErrors"`
	TotalWarnings int `json:"TotalWarnings"`
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

// ServiceCheck represents a service check emitted by a check
type ServiceCheck struct {
	Name      string   `json:"check"`
	Host      string   `json:"host_name"`
	Status    int      `json:"status"`
	Timestamp int64    `json:"timestamp"`
	Message   string   `json:"message"`
	Tags      []string `json:"tags"`
}

// Event represents a event emitted by a check
type Event struct {
	Title     string `json:"msg_title"`
	Text      string `json:"msg_text"`
	Host      string `json:"host"`
	Timestamp int64  `json:"timestamp"`
	Priority  string `json:"priority"`
	AlertType string `json:"alert_type"`
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
