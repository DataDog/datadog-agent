// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types provides common type definitions for the anomaly detection system.
package types

import "time"

// LogError represents a log error with its count and timestamp
type LogError struct {
	Message   string
	Count     int64
	Timestamp time.Time
}

// LogPattern represents a detected pattern in logs
type LogPattern struct {
	Pattern     string   // The normalized pattern
	Count       int64    // Total occurrences
	Examples    []string // Sample messages matching this pattern
	GroupingKey string   // The key used to group (e.g., filename, normalized message)
}
