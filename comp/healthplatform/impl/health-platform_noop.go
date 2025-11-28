// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package healthplatformimpl

import (
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// noopHealthPlatform is a no-op implementation of the health platform component
// Used when the health platform is disabled via configuration
type noopHealthPlatform struct{}

// ReportIssue does nothing when the health platform is disabled
func (n *noopHealthPlatform) ReportIssue(_ string, _ string, _ *healthplatform.IssueReport) error {
	return nil
}

// GetAllIssues returns empty results when the health platform is disabled
func (n *noopHealthPlatform) GetAllIssues() (int, map[string]*healthplatform.Issue) {
	return 0, make(map[string]*healthplatform.Issue)
}

// GetIssueForCheck returns nil when the health platform is disabled
func (n *noopHealthPlatform) GetIssueForCheck(_ string) *healthplatform.Issue {
	return nil
}

// ClearIssuesForCheck does nothing when the health platform is disabled
func (n *noopHealthPlatform) ClearIssuesForCheck(_ string) {
}

// ClearAllIssues does nothing when the health platform is disabled
func (n *noopHealthPlatform) ClearAllIssues() {
}
