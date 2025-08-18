// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealth provides the interface for the logs agent health checker sub-component.
// This sub-component checks for logs agent health issues and reports them to the parent health platform.
package logsagenthealth

import (
	"context"
)

// team: agent-runtimes

// Component is the logs agent health checker sub-component interface
type Component interface {
	// CheckHealth performs health checks related to logs agent health
	// and returns any issues found
	CheckHealth(ctx context.Context) ([]Issue, error)

	// Start begins periodic health checking
	Start(ctx context.Context) error

	// Stop stops periodic health checking
	Stop() error
}

// Issue represents a logs agent health issue
type Issue struct {
	// ID is the unique identifier for the issue
	ID string
	// Name is the human-readable name of the issue
	Name string
	// Extra is optional complementary information
	Extra string
	// Severity indicates the impact level of the issue
	Severity Severity
}

// Severity indicates the impact level of an issue
type Severity string

const (
	// SeverityLow indicates a minor issue with minimal impact
	SeverityLow Severity = "low"
	// SeverityMedium indicates a moderate issue that may affect performance
	SeverityMedium Severity = "medium"
	// SeverityHigh indicates a significant issue that may cause failures
	SeverityHigh Severity = "high"
	// SeverityCritical indicates a critical issue that will cause failures
	SeverityCritical Severity = "critical"
)
