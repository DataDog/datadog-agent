// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package logsmultilinejournal provides a health check for misconfigured
// multi_line rules on journald log sources, where multi_line aggregation has no effect.
package logsmultilinejournal

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/healthplatform/issues"
)

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for multi_line-on-journald issues
	IssueID = "logs-multiline-journald-unsupported"
)

// module implements issues.Module
type module struct {
	template *Issue
}

// NewModule creates a new multi_line-on-journald issue module
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

// BuiltInHealthCheck returns nil because detection is done inline by the journald
// launcher when it processes a source with multi_line rules, via ReportIssue().
func (m *module) BuiltInHealthCheck() *issues.BuiltInHealthCheck {
	return nil
}
