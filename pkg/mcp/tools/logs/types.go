// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package logs

// LogsParams defines parameters for log queries
type LogsParams struct {
	// Query is a regex pattern to filter log messages
	Query string `json:"query,omitempty"`
	// Source filters by log source
	Source string `json:"source,omitempty"`
	// Service filters by service name
	Service string `json:"service,omitempty"`
	// Name filters by source name
	Name string `json:"name,omitempty"`
	// Type filters by source type (e.g., "file", "docker", "journald")
	Type string `json:"type,omitempty"`
	// Duration is how long to collect logs in seconds (default: 10, max: 60)
	Duration int `json:"duration,omitempty"`
	// Limit is max number of log messages to return (default: 100, max: 500)
	Limit int `json:"limit,omitempty"`
}

// LogsSnapshot represents collected log messages
type LogsSnapshot struct {
	// Timestamp is the Unix timestamp when collection started
	Timestamp int64 `json:"ts"`
	// Duration is how long logs were collected in seconds
	Duration int `json:"duration"`
	// Messages contains the collected log entries
	Messages []*LogEntry `json:"messages"`
	// TotalCollected is the total number collected before limit was applied
	TotalCollected int `json:"total_collected"`
	// Count is the number of messages returned (after limit)
	Count int `json:"count"`
	// Query is the regex pattern that was used
	Query string `json:"query,omitempty"`
	// Truncated indicates if results were truncated due to limit
	Truncated bool `json:"truncated,omitempty"`
}

// LogEntry represents a single log message
type LogEntry struct {
	// Timestamp is when the log was ingested (Unix nano)
	Timestamp int64 `json:"ts"`
	// Message is the log content
	Message string `json:"msg"`
	// Status is the log level/status
	Status string `json:"status,omitempty"`
	// Source is the log source identifier
	Source string `json:"source,omitempty"`
	// Service is the service name
	Service string `json:"service,omitempty"`
	// Name is the source name
	Name string `json:"name,omitempty"`
	// Type is the source type
	Type string `json:"type,omitempty"`
	// Tags are associated tags
	Tags []string `json:"tags,omitempty"`
	// Hostname is the originating host
	Hostname string `json:"host,omitempty"`
}

// ListSourcesParams defines parameters for listing log sources
type ListSourcesParams struct {
	// Type filters by source type (e.g., "file", "docker", "journald", "tcp", "udp")
	Type string `json:"type,omitempty"`
	// Status filters by source status (e.g., "OK", "Error", "Pending")
	Status string `json:"status,omitempty"`
}

// LogSourcesList represents the list of log sources being tailed
type LogSourcesList struct {
	// Timestamp is when the list was generated
	Timestamp int64 `json:"ts"`
	// Sources contains all active log sources
	Sources []*LogSourceInfo `json:"sources"`
	// Count is the number of sources returned
	Count int `json:"count"`
	// TypeCounts shows count by source type
	TypeCounts map[string]int `json:"type_counts,omitempty"`
}

// LogSourceInfo represents a single log source being monitored
type LogSourceInfo struct {
	// Name is the integration/source name
	Name string `json:"name"`
	// Type is the source type (file, docker, journald, tcp, udp, etc.)
	Type string `json:"type"`
	// Status is the current status (OK, Error, Pending)
	Status string `json:"status"`
	// Inputs are the active inputs (file paths, container IDs, etc.)
	Inputs []string `json:"inputs,omitempty"`
	// Config contains relevant configuration
	Config *LogSourceConfig `json:"config,omitempty"`
	// BytesRead is the total bytes read from this source
	BytesRead int64 `json:"bytes_read,omitempty"`
	// Info contains additional status information
	Info map[string]string `json:"info,omitempty"`
}

// LogSourceConfig contains relevant log source configuration
type LogSourceConfig struct {
	// Path is the file path or pattern (for file sources)
	Path string `json:"path,omitempty"`
	// Service is the configured service name
	Service string `json:"service,omitempty"`
	// Source is the configured source name
	Source string `json:"source,omitempty"`
	// Tags are configured tags
	Tags []string `json:"tags,omitempty"`
	// Identifier is the source identifier
	Identifier string `json:"identifier,omitempty"`
}
