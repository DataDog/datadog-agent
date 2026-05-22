// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package issues

import (
	"fmt"
	"sync"

	"github.com/DataDog/agent-payload/v5/healthplatform"
)

// Registry manages issue modules — providing issue templates, built-in periodic
// health checks, and built-in startup health checks.
type Registry struct {
	mu             sync.RWMutex
	templates      map[string]IssueTemplate
	periodicChecks []*BuiltInPeriodicHealthCheck
	startupChecks  []*BuiltInStartupHealthCheck
}

// NewRegistry creates a new issue registry
func NewRegistry() *Registry {
	return &Registry{
		templates:      make(map[string]IssueTemplate),
		periodicChecks: make([]*BuiltInPeriodicHealthCheck, 0),
		startupChecks:  make([]*BuiltInStartupHealthCheck, 0),
	}
}

// RegisterModule registers an issue module, extracting its template, periodic
// check, and once check.
func (r *Registry) RegisterModule(module Module) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.templates[module.IssueName()] = module.IssueTemplate()

	if check := module.BuiltInPeriodicHealthCheck(); check != nil {
		check.IssueNames = append(check.IssueNames, module.IssueName())
		r.periodicChecks = append(r.periodicChecks, check)
	}
	if once := module.BuiltInStartupHealthCheck(); once != nil {
		once.IssueNames = append(once.IssueNames, module.IssueName())
		r.startupChecks = append(r.startupChecks, once)
	}
}

// GetTemplate retrieves an issue template by issue ID
func (r *Registry) GetTemplate(issueID string) (IssueTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	template, exists := r.templates[issueID]
	return template, exists
}

// BuildIssue creates a complete issue using the template and context
func (r *Registry) BuildIssue(issueType string, context map[string]string) (*healthplatform.Issue, error) {
	template, exists := r.GetTemplate(issueType)
	if !exists {
		return nil, fmt.Errorf("no issue template found for: %s", issueType)
	}
	return template.BuildIssue(context)
}

// GetBuiltInPeriodicHealthChecks returns all registered periodic health periodicChecks.
func (r *Registry) GetBuiltInPeriodicHealthChecks() []*BuiltInPeriodicHealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*BuiltInPeriodicHealthCheck, len(r.periodicChecks))
	copy(result, r.periodicChecks)
	return result
}

// GetBuiltInStartupHealthChecks returns all registered startup-time once periodicChecks.
func (r *Registry) GetBuiltInStartupHealthChecks() []*BuiltInStartupHealthCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*BuiltInStartupHealthCheck, len(r.startupChecks))
	copy(result, r.startupChecks)
	return result
}
