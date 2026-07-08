// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issueregistry defines the interface for the health platform issue registry component.
// The registry is the single source of truth for issue templates and built-in health checks.
// It is built once at startup from all registered issue modules and shared by the runner
// (for template lookup on IssueReport) and the bundle (for bootstrapping built-in checks).
package issueregistry

// team: agent-health fleet-remediation

import (
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Component is the health platform issue registry.
type Component interface {
	// GetTemplate returns the Template registered for issueName, or false if none is registered.
	GetTemplate(issueName string) (issuesmod.Template, bool)

	// GetBuiltInPeriodicHealthChecks returns all registered periodic health checks.
	GetBuiltInPeriodicHealthChecks() []*runnerdef.BuiltInPeriodicHealthCheck

	// GetBuiltInStartupHealthChecks returns all registered once-at-startup health checks.
	GetBuiltInStartupHealthChecks() []*runnerdef.BuiltInHealthCheck
}
