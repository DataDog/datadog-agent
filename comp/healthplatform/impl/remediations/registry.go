// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package remediations provides a registry of remediation templates for known issues.
// Integrations report issues by ID with context, and the registry fills in OS-specific remediations.
package remediations

import (
	"fmt"
	"sync"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/remediations/dockerpermissions"
)

// IssueTemplate defines how to build a complete issue (metadata + remediation) from context
type IssueTemplate interface {
	// BuildIssue creates a complete issue using the provided context
	BuildIssue(context map[string]string) *healthplatform.Issue
}

// Registry manages issue templates for known issues
type Registry struct {
	mu        sync.RWMutex
	templates map[string]IssueTemplate
}

// NewRegistry creates a new issue registry with built-in templates
func NewRegistry() *Registry {
	r := &Registry{
		templates: make(map[string]IssueTemplate),
	}

	// Register built-in issue templates
	r.registerBuiltInTemplates()

	return r
}

// registerBuiltInTemplates registers all built-in issue templates
func (r *Registry) registerBuiltInTemplates() {
	// Docker log permissions
	r.Register("docker-file-tailing-disabled", dockerpermissions.NewDockerPermissionIssue())
}

// Register adds an issue template for a specific issue ID
func (r *Registry) Register(issueID string, template IssueTemplate) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.templates[issueID] = template
}

// Get retrieves an issue template by issue ID
func (r *Registry) Get(issueID string) (IssueTemplate, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	template, exists := r.templates[issueID]
	return template, exists
}

// BuildIssue creates a complete issue using the template and context
func (r *Registry) BuildIssue(issueID string, context map[string]string) (*healthplatform.Issue, error) {
	template, exists := r.Get(issueID)
	if !exists {
		return nil, fmt.Errorf("no issue template found for: %s", issueID)
	}

	return template.BuildIssue(context), nil
}
