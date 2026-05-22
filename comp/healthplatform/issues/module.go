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
// 3. Call RegisterModuleFactory in your package's init() function
package issues

import (
	"sync"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// ModuleFactory is a function that creates a new Module instance
type ModuleFactory func(config config.Component) Module

var (
	moduleFactories   []ModuleFactory
	moduleFactoriesMu sync.Mutex
)

// RegisterModuleFactory registers a module factory function.
// This should be called from the module's init() function.
func RegisterModuleFactory(factory ModuleFactory) {
	moduleFactoriesMu.Lock()
	defer moduleFactoriesMu.Unlock()
	moduleFactories = append(moduleFactories, factory)
}

// GetAllModules creates and returns all registered modules.
// Each call creates new module instances.
func GetAllModules(config config.Component) []Module {
	moduleFactoriesMu.Lock()
	defer moduleFactoriesMu.Unlock()

	modules := make([]Module, 0, len(moduleFactories))
	for _, factory := range moduleFactories {
		modules = append(modules, factory(config))
	}
	return modules
}

// IssueTemplate defines how to build a complete issue (metadata + remediation) from context
type IssueTemplate interface {
	// BuildIssue creates a complete issue using the provided context.
	BuildIssue(context map[string]string) (*healthplatform.Issue, error)
}

// BuiltInPeriodicHealthCheck represents configuration for a periodic built-in health check.
// Source is the reporting component label passed to scheduler.Schedule.
// Fn returns zero or more IssueReports; returning nil/empty means no issue detected.
// IssueNames is populated automatically by Registry.RegisterModule from module.IssueName();
// module authors must not set it. bundle.go uses it to query the store for persisted
// issues from a prior run so the scheduler can resolve them after restart.
type BuiltInPeriodicHealthCheck struct {
	Source     string
	Fn         runnerdef.HealthCheckFunc
	Interval   time.Duration
	IssueNames []string
}

// BuiltInStartupHealthCheck represents a check that runs exactly once at agent startup via
// runner.Run. Use this for startup-time diagnostics (e.g. filesystem permissions)
// where a periodic poll makes no sense.
// IssueNames is populated automatically by Registry.RegisterModule; see BuiltInPeriodicHealthCheck.
type BuiltInStartupHealthCheck struct {
	Source     string
	Fn         runnerdef.HealthCheckFunc
	IssueNames []string
}

// Module represents a complete issue feature module.
// Each module bundles detection (optional) with remediation.
type Module interface {
	// IssueName returns the snake_case issue name for this module. It is the
	// key used to look up the issue template in the registry and must equal the
	// IssueName field in any proto Issue emitted by this module's checks.
	IssueName() string

	// IssueTemplate returns the template for building complete issues.
	IssueTemplate() IssueTemplate

	// BuiltInPeriodicHealthCheck returns the periodic health check configuration, or nil
	// if this module has no periodic check.
	BuiltInPeriodicHealthCheck() *BuiltInPeriodicHealthCheck

	// BuiltInStartupHealthCheck returns a check that runs once at startup, or nil if
	// this module has no startup-time check.
	BuiltInStartupHealthCheck() *BuiltInStartupHealthCheck
}
