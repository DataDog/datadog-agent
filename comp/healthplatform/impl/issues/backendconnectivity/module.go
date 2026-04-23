// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package backendconnectivity provides a complete issue module for Datadog backend connectivity failures.
// It includes both detection (built-in health check) and remediation (issue template with fix steps).
package backendconnectivity

import (
	"github.com/DataDog/agent-payload/v5/healthplatform"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for backend connectivity issues
	IssueID = "backend-connectivity-failure"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "backend-connectivity"

	// CheckName is the human-readable name for the health check
	CheckName = "Backend Connectivity"
)

// backendConnectivityModule implements issues.Module
type backendConnectivityModule struct {
	cfg      config.Component
	template *BackendConnectivityIssue
}

// NewModule creates a new backend connectivity issue module
func NewModule(cfg config.Component) issues.Module {
	return &backendConnectivityModule{
		cfg:      cfg,
		template: NewBackendConnectivityIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *backendConnectivityModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *backendConnectivityModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns the built-in health check configuration
// Interval is 0 to use the default (15 minutes)
func (m *backendConnectivityModule) BuiltInCheck() *issues.BuiltInCheck {
	return &issues.BuiltInCheck{
		ID:   CheckID,
		Name: CheckName,
		CheckFn: func() (*healthplatform.IssueReport, error) {
			return Check(m.cfg)
		},
	}
}
