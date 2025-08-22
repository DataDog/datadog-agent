// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsagenthealth provides the interface for the logs agent health checker sub-component.
// This sub-component checks for logs agent health issues and reports them to the parent health platform.
package logsagenthealth

import (
	"context"

	healthplatform "github.com/DataDog/datadog-agent/comp/core/health-platform/def"
)

// team: agent-runtimes

// SubCheck represents a single health check that can be registered with the logs agent health component
type SubCheck interface {
	// Check performs a single health check and returns any issues found
	Check(ctx context.Context) ([]healthplatform.Issue, error)
	// Name returns the name of this sub-check
	Name() string
}

// Component is the logs agent health checker sub-component interface
type Component interface {
	// CheckHealth performs health checks related to logs agent health
	// and returns any issues found
	CheckHealth(ctx context.Context) ([]healthplatform.Issue, error)

	// RegisterSubCheck registers a new health check sub-component
	RegisterSubCheck(check SubCheck) error
}
