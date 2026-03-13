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

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/def"
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
	// BuildIssue creates a complete issue using the provided context
	BuildIssue(context map[string]string) (*healthplatform.Issue, error)
}

// BuiltInCheck represents configuration for a built-in health check
type BuiltInCheck struct {
	ID       string
	Name     string
	CheckFn  healthplatformdef.HealthCheckFunc
	Interval time.Duration

	// Once is mutually exclusive with Interval.
	// If true, the check will only run once at startup.
	Once bool
}

// Module represents a complete issue feature module
// Each module bundles detection (optional) with remediation
type Module interface {
	// IssueID returns the unique identifier for this issue type
	IssueID() string

	// IssueTemplate returns the template for building complete issues
	IssueTemplate() IssueTemplate

	// BuiltInCheck returns the built-in health check configuration, or nil if
	// this issue is only reported by external integrations
	BuiltInCheck() *BuiltInCheck
}
