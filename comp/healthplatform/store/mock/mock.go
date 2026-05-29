// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the health-platform component
package mock

import (
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

type mockHealthPlatform struct {
	issues map[string]*healthplatformpayload.Issue
}

// Mock returns a mock health platform component for testing
func Mock(_ *testing.T) healthplatform.Component {
	return &mockHealthPlatform{
		issues: make(map[string]*healthplatformpayload.Issue),
	}
}

// ReportIssue stores the proto Issue keyed by issue.Id for testing.
func (m *mockHealthPlatform) ReportIssue(issue *healthplatformpayload.Issue) error {
	if issue == nil || issue.Id == "" {
		return nil
	}
	m.issues[issue.Id] = proto.Clone(issue).(*healthplatformpayload.Issue)
	return nil
}

// GetAllIssues returns the count and all issues from all checks
func (m *mockHealthPlatform) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	count := 0
	result := make(map[string]*healthplatformpayload.Issue)
	for checkID, issue := range m.issues {
		if issue != nil {
			result[checkID] = proto.Clone(issue).(*healthplatformpayload.Issue)
			count++
		} else {
			result[checkID] = nil
		}
	}
	return count, result
}

// GetIssue returns the issue for a specific check
func (m *mockHealthPlatform) GetIssue(checkID string) *healthplatformpayload.Issue {
	issue := m.issues[checkID]
	if issue == nil {
		return nil
	}
	return proto.Clone(issue).(*healthplatformpayload.Issue)
}

// ResolveIssue clears issues for a specific check
func (m *mockHealthPlatform) ResolveIssue(checkID string) {
	delete(m.issues, checkID)
}

// ResolveAllIssues clears all issues
func (m *mockHealthPlatform) ResolveAllIssues() {
	m.issues = make(map[string]*healthplatformpayload.Issue)
}

// GetActiveIssueIDsByIssueName returns nil in the mock.
func (m *mockHealthPlatform) GetActiveIssueIDsByIssueName(_ string) []string {
	return nil
}
