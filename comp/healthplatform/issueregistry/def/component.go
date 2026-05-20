// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package issueregistry defines the interface for the health platform issue registry component.
// The registry is the single source of truth for issue templates and built-in health checks.
// It is built once at startup from all registered issue modules and shared by the store
// (for template lookup on ReportIssue) and the bundle (for bootstrapping built-in checks).
package issueregistry

// team: agent-health

import (
	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	issuesmod "github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

// Component is the health platform issue registry.
type Component interface {
	// BuildIssue builds a complete proto Issue from the template registered for
	// issueType, filling in template variables from context.
	// Returns an error if no template is registered for issueType.
	BuildIssue(issueType string, context map[string]string) (*healthplatformpayload.Issue, error)

	// HasTemplate reports whether a template is registered for issueType.
	HasTemplate(issueType string) bool

	// GetBuiltInPeriodicHealthChecks returns all registered periodic health checks.
	// TODO: move BuiltInPeriodicHealthCheck / BuiltInStartupHealthCheck into this def
	// package to avoid coupling the interface to the issues implementation package.
	GetBuiltInPeriodicHealthChecks() []*issuesmod.BuiltInPeriodicHealthCheck

	// GetBuiltInStartupHealthChecks returns all registered once-at-startup health checks.
	GetBuiltInStartupHealthChecks() []*issuesmod.BuiltInStartupHealthCheck
}
