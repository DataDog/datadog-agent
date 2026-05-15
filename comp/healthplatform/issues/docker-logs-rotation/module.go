// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dockerlogsrotation provides the issue module for Docker container log rotation risk.
package dockerlogsrotation

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for Docker log rotation risk issues
	IssueID = "docker-logs-rotation-risk"
)

// module implements issues.Module
type module struct {
	template *Issue
}

// NewModule creates a new Docker logs rotation issue module
func NewModule(_ config.Component) issues.Module {
	return &module{template: NewIssue()}
}

// IssueID returns the unique identifier for this issue type
func (m *module) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *module) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInHealthCheck returns nil because detection happens in-workflow when the
// Docker socket tailer is initialized in socket-based mode.
func (m *module) BuiltInHealthCheck() *issues.BuiltInHealthCheck {
	return nil
}
