// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logshosttagsmissing detects when logs-only mode is configured without host tags.
package logshosttagsmissing

import (
	"github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for logs host tags missing issues
	IssueID = "logs-host-tags-missing"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "logs-host-tags-config"

	// CheckName is the human-readable name for the health check
	CheckName = "Logs-Only Agent Host Tags Configuration"
)

// logsHostTagsMissingModule implements issues.Module
type logsHostTagsMissingModule struct {
	template *Issue
}

// NewModule creates a new logs host tags missing issue module
func NewModule() issues.Module {
	return &logsHostTagsMissingModule{
		template: NewIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *logsHostTagsMissingModule) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *logsHostTagsMissingModule) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil because detection is performed inline at logs agent
// startup via healthplatform.ReportIssue(), not by a polling check.
func (m *logsHostTagsMissingModule) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
