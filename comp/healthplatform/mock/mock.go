// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package mock provides a mock implementation of the health-platform component
package mock

import (
	"testing"
	"time"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
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

// ReportIssue reports an issue with minimal information
// The mock implementation just creates and stores a basic issue
// If report is nil, it clears the issue (resolution)
func (m *mockHealthPlatform) ReportIssue(checkID string, checkName string, report *healthplatformpayload.IssueReport) error {
	if checkID == "" {
		return nil // Mock doesn't validate
	}

	// If report is nil, clear the issue
	if report == nil {
		delete(m.issues, checkID)
		return nil
	}

	// Create a basic issue from the report for testing purposes
	issue := &healthplatformpayload.Issue{
		Id:          report.IssueId,
		Title:       checkName,
		Description: "Mock issue",
		Category:    "test",
		Location:    "test",
		Severity:    "low",
		Source:      "test",
		Tags:        report.Tags,
	}

	// Store the issue
	m.issues[checkID] = issue

	return nil
}

// RegisterCheck does nothing in the mock implementation
func (m *mockHealthPlatform) RegisterCheck(_ string, _ string, _ healthplatform.HealthCheckFunc, _ time.Duration) error {
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

// GetIssueForCheck returns the issue for a specific check
func (m *mockHealthPlatform) GetIssueForCheck(checkID string) *healthplatformpayload.Issue {
	issue := m.issues[checkID]
	if issue == nil {
		return nil
	}
	return proto.Clone(issue).(*healthplatformpayload.Issue)
}

// ClearIssuesForCheck clears issues for a specific check
func (m *mockHealthPlatform) ClearIssuesForCheck(checkID string) {
	delete(m.issues, checkID)
}

// ClearAllIssues clears all issues
func (m *mockHealthPlatform) ClearAllIssues() {
	m.issues = make(map[string]*healthplatformpayload.Issue)
}
