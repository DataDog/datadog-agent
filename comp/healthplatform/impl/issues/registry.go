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

// Registry manages issue modules - providing both issue templates and built-in checks
type Registry struct {
	mu        sync.RWMutex
	templates map[string]IssueTemplate // issueID -> template
	checks    []*BuiltInCheck          // all built-in checks
}

// NewRegistry creates a new issue registry
func NewRegistry() *Registry {
	return &Registry{
		templates: make(map[string]IssueTemplate),
		checks:    make([]*BuiltInCheck, 0),
	}
}

// RegisterModule registers an issue module, extracting both the template and optional check
func (r *Registry) RegisterModule(module Module) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Register the issue template
	r.templates[module.IssueID()] = module.IssueTemplate()

	// Register the built-in check if present
	if check := module.BuiltInCheck(); check != nil {
		r.checks = append(r.checks, check)
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
func (r *Registry) BuildIssue(issueID string, context map[string]string) (*healthplatform.Issue, error) {
	template, exists := r.GetTemplate(issueID)
	if !exists {
		return nil, fmt.Errorf("no issue template found for: %s", issueID)
	}

	return template.BuildIssue(context)
}

// GetBuiltInChecks returns all built-in health checks from registered modules
func (r *Registry) GetBuiltInChecks() []*BuiltInCheck {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to avoid external modifications
	result := make([]*BuiltInCheck, len(r.checks))
	copy(result, r.checks)
	return result
}
