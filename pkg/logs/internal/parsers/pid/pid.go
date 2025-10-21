// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package pid provides utilities for extracting process IDs from log messages
package pid

import (
	"encoding/json"
	"regexp"
	"strconv"
)

var (
	// Regex patterns for extracting PID from various log formats

	// Matches syslog RFC3164 format: "process[1234]:" or "process[1234] "
	syslogRFC3164Pattern = regexp.MustCompile(`\[(\d+)\][\s:]`)

	// Matches common PID patterns: "pid=1234", "pid:1234", "pid 1234", "[pid: 1234]", "(pid 1234)"
	commonPIDPatterns = regexp.MustCompile(`(?i)(?:^|\s|[\[\(])pid[\s:=]+(\d+)`)

	// Matches process_id patterns: "process_id=1234", "process_id:1234", "ProcessID=1234"
	processIDPattern = regexp.MustCompile(`(?i)process[_\s]?id[\s:=]+(\d+)`)
)

// ExtractPID attempts to extract a PID from a log message using multiple strategies.
// It tries JSON parsing first, then syslog formats, then common patterns.
// Returns the PID as int32, or 0 if no PID could be extracted.
func ExtractPID(content []byte) int32 {
	// Try JSON extraction first (most structured)
	if pid := extractFromJSON(content); pid > 0 {
		return pid
	}

	// Try syslog format
	if pid := extractFromSyslog(content); pid > 0 {
		return pid
	}

	// Try common patterns
	if pid := extractFromPatterns(content); pid > 0 {
		return pid
	}

	return 0
}

// extractFromJSON attempts to extract PID from JSON-formatted logs.
// Looks for fields: "pid", "process_id", "processId", "PID"
func extractFromJSON(content []byte) int32 {
	var data map[string]interface{}
	if err := json.Unmarshal(content, &data); err != nil {
		return 0
	}

	// Check common JSON field names for PID
	fieldNames := []string{"pid", "process_id", "processId", "PID", "ProcessID"}

	for _, fieldName := range fieldNames {
		if pidValue, exists := data[fieldName]; exists {
			return convertToInt32(pidValue)
		}
	}

	return 0
}

// extractFromSyslog attempts to extract PID from syslog-formatted messages.
// Matches RFC3164 format like: "process[1234]: message"
func extractFromSyslog(content []byte) int32 {
	matches := syslogRFC3164Pattern.FindSubmatch(content)
	if len(matches) >= 2 {
		if pid, err := strconv.ParseInt(string(matches[1]), 10, 32); err == nil {
			return int32(pid)
		}
	}
	return 0
}

// extractFromPatterns attempts to extract PID from common patterns in log messages.
// Matches patterns like: "pid=1234", "[pid: 1234]", "process_id=1234"
func extractFromPatterns(content []byte) int32 {
	// Try common PID patterns first
	matches := commonPIDPatterns.FindSubmatch(content)
	if len(matches) >= 2 {
		if pid, err := strconv.ParseInt(string(matches[1]), 10, 32); err == nil {
			return int32(pid)
		}
	}

	// Try process_id patterns
	matches = processIDPattern.FindSubmatch(content)
	if len(matches) >= 2 {
		if pid, err := strconv.ParseInt(string(matches[1]), 10, 32); err == nil {
			return int32(pid)
		}
	}

	return 0
}

// convertToInt32 converts various types to int32 for PID values
func convertToInt32(value interface{}) int32 {
	switch v := value.(type) {
	case float64:
		return int32(v)
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	case string:
		if pid, err := strconv.ParseInt(v, 10, 32); err == nil {
			return int32(pid)
		}
	}
	return 0
}
