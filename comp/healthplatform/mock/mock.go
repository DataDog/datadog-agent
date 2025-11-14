// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health-platform component
package mock

import (
	"testing"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/def"
)

// mockHealthPlatform is a mock implementation of the health-platform component
// It provides in-memory storage for testing purposes without running actual health checks
type mockHealthPlatform struct {
	checks map[string]healthplatform.CheckConfig
	issues map[string]*healthplatform.Issue
}

// Mock returns a mock for health-platform component.
// The mock provides a simple in-memory implementation suitable for testing
func Mock(_ *testing.T) healthplatform.Component {
	return &mockHealthPlatform{
		checks: make(map[string]healthplatform.CheckConfig),
		issues: make(map[string]*healthplatform.Issue),
	}
}

// RegisterCheck registers a health check with the mock platform
func (m *mockHealthPlatform) RegisterCheck(check healthplatform.CheckConfig) error {
	if check.CheckID == "" {
		return nil // Mock doesn't validate, just stores
	}
	m.checks[check.CheckID] = check
	return nil
}

// GetAllIssues returns the count and all issues from all checks
func (m *mockHealthPlatform) GetAllIssues() (int, map[string]*healthplatform.Issue) {
	count := 0
	result := make(map[string]*healthplatform.Issue)
	for checkID, issue := range m.issues {
		if issue != nil {
			issueCopy := *issue
			result[checkID] = &issueCopy
			count++
		} else {
			result[checkID] = nil
		}
	}
	return count, result
}

// GetIssueForCheck returns the issue for a specific check
func (m *mockHealthPlatform) GetIssueForCheck(checkID string) *healthplatform.Issue {
	issue := m.issues[checkID]
	if issue == nil {
		return nil
	}
	issueCopy := *issue
	return &issueCopy
}

// ClearIssuesForCheck clears issues for a specific check
func (m *mockHealthPlatform) ClearIssuesForCheck(checkID string) {
	delete(m.issues, checkID)
}

// ClearAllIssues clears all issues
func (m *mockHealthPlatform) ClearAllIssues() {
	m.issues = make(map[string]*healthplatform.Issue)
}

// RunHealthChecks manually triggers health check execution
// If async is true, checks run in parallel goroutines
// If async is false, checks run synchronously (useful for testing)
func (m *mockHealthPlatform) RunHealthChecks(async bool) {
	// Mock implementation - no-op since mock doesn't run actual health checks
	_ = async
}

// ReportIssue reports an issue with minimal information
// The mock implementation just creates and stores a basic issue
// If report is nil, it clears the issue (resolution)
func (m *mockHealthPlatform) ReportIssue(checkID string, checkName string, report *healthplatform.IssueReport) error {
	if checkID == "" {
		return nil // Mock doesn't validate
	}

	// If report is nil, clear the issue
	if report == nil {
		delete(m.issues, checkID)
		return nil
	}

	// Create a basic issue from the report for testing purposes
	issue := &healthplatform.Issue{
		ID:                 report.IssueID,
		Title:              checkName,
		Description:        "Mock issue",
		Category:           "test",
		Location:           "test",
		Severity:           "low",
		IntegrationFeature: "test",
		Tags:               report.Tags,
	}

	// Store the issue
	m.issues[checkID] = issue

	return nil
}
