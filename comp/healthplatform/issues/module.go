// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issues provides feature modules that bundle health checks with their remediations.
// Each sub-package represents a complete "issue module" containing:
// - Detection logic (optional built-in health check)
// - Remediation templates (issue metadata, description, fix steps, scripts)
//
// To add a new issue module:
// 1. Create a new sub-package (e.g., issues/myissue/)
// 2. Implement the Module interface
// 3. Add Module() providing an issues.Module field tagged group:"healthplatform_issue" through fxutil.ProvideComponentConstructor.
// 4. Add the package's Module() to bundle.go.
// 5. Return an empty Provides (nil member) to opt out; config is available in the constructor.
//
// Health-check IssueIDs must be unique per host, since a downstream aggregator
// keys recommendations on (org, IssueID) alone. A module whose check can run
// with different results/config on multiple hosts (or multiple binaries on the
// same host) must scope its IssueID accordingly — see invalidconfig's
// instanceIssueID for the pattern.
package issues

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// Template is the remediation side of a Module: it knows its issue name and
// can build a complete Issue from context.
type Template interface {
	// IssueName returns the issue name. It is the registry key and
	// must equal the IssueName field in any proto Issue emitted by this module's checks.
	IssueName() string

	// IssueType returns the issue type. It must equal the IssueType field in any
	// proto Issue emitted by this module's checks, and must equal IssueName()
	// lowercased with spaces replaced by underscores (hyphens preserved).
	IssueType() string

	// BuildIssue creates a complete issue using the provided context.
	BuildIssue(context map[string]string) (*healthplatform.Issue, error)
}

// HealthCheckProvider is the detection side of a Module.
// Both methods return nil if this module has no check of that type.
type HealthCheckProvider interface {
	// BuiltInPeriodicHealthCheck returns the periodic health check configuration, or nil.
	BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck

	// BuiltInStartupHealthCheck returns a check that runs once at startup, or nil.
	BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck
}

// Module bundles detection (optional) with remediation for a single issue type.
type Module interface {
	Template
	HealthCheckProvider
}
