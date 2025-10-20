// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package def defines the doctor component interface
package def

import (
	"time"
)

// team: agent-runtimes

// Component is the doctor component interface
// The doctor component aggregates agent telemetry and health information
// to provide a troubleshooting dashboard view of the agent's state
type Component interface {
	// GetStatus returns the current doctor status aggregating all telemetry
	GetStatus() *DoctorStatus
}

// DoctorStatus is the root structure containing all agent status information
type DoctorStatus struct {
	Timestamp time.Time       `json:"timestamp"`
	Ingestion IngestionStatus `json:"ingestion"`
	Agent     AgentStatus     `json:"agent"`
	Intake    IntakeStatus    `json:"intake"`
}

// IngestionStatus represents data collection status from various sources
type IngestionStatus struct {
	Checks    ChecksStatus    `json:"checks"`
	DogStatsD DogStatsDStatus `json:"dogstatsd"`
	Logs      LogsStatus      `json:"logs"`
	Metrics   MetricsStatus   `json:"metrics"`
}

// ChecksStatus represents the status of agent checks
type ChecksStatus struct {
	Total     int         `json:"total"`
	Running   int         `json:"running"`
	Errors    int         `json:"errors"`
	Warnings  int         `json:"warnings"`
	CheckList []CheckInfo `json:"check_list"`
}

// CheckInfo contains details about a specific check
type CheckInfo struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"` // "ok", "warning", "error"
	LastRun      time.Time `json:"last_run"`
	LastError    string    `json:"last_error,omitempty"`
	MetricsCount int       `json:"metrics_count"`
}

// DogStatsDStatus represents DogStatsD metrics ingestion status
type DogStatsDStatus struct {
	MetricsReceived int64 `json:"metrics_received"`
	PacketsReceived int64 `json:"packets_received"`
	PacketsDropped  int64 `json:"packets_dropped"`
	ParseErrors     int64 `json:"parse_errors"`
}

// LogsStatus represents log collection status
type LogsStatus struct {
	Enabled        bool        `json:"enabled"`
	Sources        int         `json:"sources"`
	BytesProcessed int64       `json:"bytes_processed"`
	LinesProcessed int64       `json:"lines_processed"`
	Errors         int         `json:"errors"`
	Integrations   []LogSource `json:"integrations"` // Detailed source information
}

// LogSource represents a single log source
type LogSource struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Status string            `json:"status"` // "pending", "success", "error"
	Inputs []string          `json:"inputs"` // File paths being tailed
	Info   map[string]string `json:"info"`   // Stats (bytes read, latency, etc.)
}

// MetricsStatus represents metrics aggregation status
type MetricsStatus struct {
	InQueue int   `json:"in_queue"`
	Flushed int64 `json:"flushed"`
}

// AgentStatus represents agent health and metadata
type AgentStatus struct {
	Running        bool          `json:"running"`
	Version        string        `json:"version"`
	Hostname       string        `json:"hostname"`
	Uptime         time.Duration `json:"uptime"`
	ErrorsLast5Min int           `json:"errors_last_5min"`
	Health         HealthStatus  `json:"health"`
	Tags           []string      `json:"tags"`
}

// HealthStatus represents the health state of agent components
type HealthStatus struct {
	Healthy   []string `json:"healthy"`
	Unhealthy []string `json:"unhealthy"`
}

// IntakeStatus represents backend connectivity and data forwarding status
type IntakeStatus struct {
	Connected  bool             `json:"connected"`
	APIKeyInfo APIKeyInfo       `json:"api_key_info"`
	LastFlush  time.Time        `json:"last_flush"`
	RetryQueue int              `json:"retry_queue_size"`
	Endpoints  []EndpointStatus `json:"endpoints"`
}

// APIKeyInfo contains information about the API key
type APIKeyInfo struct {
	Valid         bool      `json:"valid"`
	LastValidated time.Time `json:"last_validated,omitempty"`
}

// EndpointStatus represents the status of a specific intake endpoint
type EndpointStatus struct {
	Name      string `json:"name"` // "metrics", "logs", "traces"
	URL       string `json:"url"`
	Status    string `json:"status"` // "connected", "error", "unknown"
	LastError string `json:"last_error,omitempty"`
}
