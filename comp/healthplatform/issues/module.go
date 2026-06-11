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

// Template is the remediation side of a Module: it knows its issue name and
// can build a complete Issue from context.
type Template interface {
	// IssueName returns the issue name. It is the registry key and
	// must equal the IssueName field in any proto Issue emitted by this module's checks.
	IssueName() string

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
