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
//
// Health-check IssueIDs must be unique per host, since a downstream aggregator
// keys recommendations on (org, IssueID) alone. A module whose check can run
// with different results/config on multiple hosts (or multiple binaries on the
// same host) must scope its IssueID accordingly — see invalidconfig's
// instanceIssueID for the pattern.
package issues

import (
	"sync"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	sysprobeconfig "github.com/DataDog/datadog-agent/comp/core/sysprobeconfig/def"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// ModuleDeps carries the dependencies available to every issue module.
// SysProbeConfig is optional: it is nil in commands that don't bundle system-probe config.
type ModuleDeps struct {
	Config         config.Component
	SysProbeConfig sysprobeconfig.Component
	Hostname       hostnameinterface.Component
}

// ModuleFactory is a function that creates a new Module instance
type ModuleFactory func(deps ModuleDeps) Module

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
func GetAllModules(deps ModuleDeps) []Module {
	moduleFactoriesMu.Lock()
	defer moduleFactoriesMu.Unlock()

	modules := make([]Module, 0, len(moduleFactories))
	for _, factory := range moduleFactories {
		modules = append(modules, factory(deps))
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
