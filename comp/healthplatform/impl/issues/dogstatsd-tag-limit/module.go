// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package dogstatsdtaglimit provides a health platform issue module that detects
// when DogStatsD metrics may be silently dropped due to the tag count limit.
package dogstatsdtaglimit

import "github.com/DataDog/datadog-agent/comp/healthplatform/impl/issues"

func init() {
	issues.RegisterModuleFactory(NewModule)
}

const (
	// IssueID is the unique identifier for the DogStatsD tag count limit drop issue
	IssueID = "dogstatsd-tag-limit-drop"

	// CheckID is the unique identifier for the built-in health check
	CheckID = "dogstatsd-tag-limit-config"

	// CheckName is the human-readable name for the health check
	CheckName = "DogStatsD Tag Count Limit"

	// defaultMaxTagsCount is the fallback value used in issue templates when
	// the actual limit is not provided in the context.
	defaultMaxTagsCount = 100
)

// module implements issues.Module
type module struct {
	template *Issue
}

// NewModule creates a new DogStatsD tag count limit issue module
func NewModule() issues.Module {
	return &module{
		template: NewIssue(),
	}
}

// IssueID returns the unique identifier for this issue type
func (m *module) IssueID() string {
	return IssueID
}

// IssueTemplate returns the template for building complete issues
func (m *module) IssueTemplate() issues.IssueTemplate {
	return m.template
}

// BuiltInCheck returns nil - tag limit drops are detected in-workflow by the DogStatsD server
// via ReportIssue when metrics are actually truncated.
func (m *module) BuiltInCheck() *issues.BuiltInCheck {
	return nil
}
