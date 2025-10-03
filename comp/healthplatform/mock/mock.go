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
	issues map[string][]healthplatform.Issue
}

// Mock returns a mock for health-platform component.
// The mock provides a simple in-memory implementation suitable for testing
func Mock(_ *testing.T) healthplatform.Component {
	return &mockHealthPlatform{
		checks: make(map[string]healthplatform.CheckConfig),
		issues: make(map[string][]healthplatform.Issue),
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

// GetAllIssues returns all issues from all checks
func (m *mockHealthPlatform) GetAllIssues() map[string][]healthplatform.Issue {
	result := make(map[string][]healthplatform.Issue)
	for checkID, issues := range m.issues {
		result[checkID] = make([]healthplatform.Issue, len(issues))
		copy(result[checkID], issues)
	}
	return result
}

// GetIssuesForCheck returns issues for a specific check
func (m *mockHealthPlatform) GetIssuesForCheck(checkID string) []healthplatform.Issue {
	issues, exists := m.issues[checkID]
	if !exists {
		return []healthplatform.Issue{}
	}
	result := make([]healthplatform.Issue, len(issues))
	copy(result, issues)
	return result
}

// GetTotalIssueCount returns the total number of issues across all checks
func (m *mockHealthPlatform) GetTotalIssueCount() int {
	total := 0
	for _, issues := range m.issues {
		total += len(issues)
	}
	return total
}

// ClearIssuesForCheck clears issues for a specific check
func (m *mockHealthPlatform) ClearIssuesForCheck(checkID string) {
	delete(m.issues, checkID)
}

// ClearAllIssues clears all issues
func (m *mockHealthPlatform) ClearAllIssues() {
	m.issues = make(map[string][]healthplatform.Issue)
}

// RunHealthChecksNow manually triggers health check execution (useful for testing)
func (m *mockHealthPlatform) RunHealthChecksNow() {
	// Mock implementation - no-op since mock doesn't run actual health checks
}
