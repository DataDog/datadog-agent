// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package issues

import (
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockIssueTemplate is a test implementation of IssueTemplate
type mockIssueTemplate struct {
	issueID string
}

func (m *mockIssueTemplate) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Id:          m.issueID,
		Title:       "Test Issue: " + m.issueID,
		Description: "Context value: " + context["key"],
		Severity:    "medium",
	}, nil
}

// mockModuleWithCheck is a test module that has a built-in check
type mockModuleWithCheck struct {
	id       string
	template *mockIssueTemplate
}

func (m *mockModuleWithCheck) IssueID() string {
	return m.id
}

func (m *mockModuleWithCheck) IssueTemplate() IssueTemplate {
	return m.template
}

func (m *mockModuleWithCheck) BuiltInCheck() *BuiltInCheck {
	return &BuiltInCheck{
		ID:       "check-" + m.id,
		Name:     "Check for " + m.id,
		CheckFn:  func() (*healthplatform.IssueReport, error) { return nil, nil },
		Interval: 5 * time.Minute,
	}
}

// mockModuleWithoutCheck is a test module that has no built-in check (remediation only)
type mockModuleWithoutCheck struct {
	id       string
	template *mockIssueTemplate
}

func (m *mockModuleWithoutCheck) IssueID() string {
	return m.id
}

func (m *mockModuleWithoutCheck) IssueTemplate() IssueTemplate {
	return m.template
}

func (m *mockModuleWithoutCheck) BuiltInCheck() *BuiltInCheck {
	return nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()

	assert.NotNil(t, registry)
	assert.NotNil(t, registry.templates)
	assert.NotNil(t, registry.checks)
	assert.Empty(t, registry.templates)
	assert.Empty(t, registry.checks)
}

func TestRegisterModuleWithCheck(t *testing.T) {
	registry := NewRegistry()
	module := &mockModuleWithCheck{
		id:       "test-issue-1",
		template: &mockIssueTemplate{issueID: "test-issue-1"},
	}

	registry.RegisterModule(module)

	// Verify template was registered
	template, exists := registry.GetTemplate("test-issue-1")
	assert.True(t, exists)
	assert.NotNil(t, template)

	// Verify check was registered
	checks := registry.GetBuiltInChecks()
	assert.Len(t, checks, 1)
	assert.Equal(t, "check-test-issue-1", checks[0].ID)
	assert.Equal(t, "Check for test-issue-1", checks[0].Name)
	assert.Equal(t, 5*time.Minute, checks[0].Interval)
}

func TestRegisterModuleWithoutCheck(t *testing.T) {
	registry := NewRegistry()
	module := &mockModuleWithoutCheck{
		id:       "test-issue-2",
		template: &mockIssueTemplate{issueID: "test-issue-2"},
	}

	registry.RegisterModule(module)

	// Verify template was registered
	template, exists := registry.GetTemplate("test-issue-2")
	assert.True(t, exists)
	assert.NotNil(t, template)

	// Verify no check was registered
	checks := registry.GetBuiltInChecks()
	assert.Empty(t, checks)
}

func TestRegisterMultipleModules(t *testing.T) {
	registry := NewRegistry()

	// Register module with check
	registry.RegisterModule(&mockModuleWithCheck{
		id:       "issue-with-check",
		template: &mockIssueTemplate{issueID: "issue-with-check"},
	})

	// Register module without check
	registry.RegisterModule(&mockModuleWithoutCheck{
		id:       "issue-without-check",
		template: &mockIssueTemplate{issueID: "issue-without-check"},
	})

	// Register another module with check
	registry.RegisterModule(&mockModuleWithCheck{
		id:       "another-issue-with-check",
		template: &mockIssueTemplate{issueID: "another-issue-with-check"},
	})

	// Verify all templates registered
	_, exists1 := registry.GetTemplate("issue-with-check")
	_, exists2 := registry.GetTemplate("issue-without-check")
	_, exists3 := registry.GetTemplate("another-issue-with-check")
	assert.True(t, exists1)
	assert.True(t, exists2)
	assert.True(t, exists3)

	// Verify only modules with checks are in the checks list
	checks := registry.GetBuiltInChecks()
	assert.Len(t, checks, 2)
}

func TestGetTemplateNotFound(t *testing.T) {
	registry := NewRegistry()

	template, exists := registry.GetTemplate("non-existent-issue")

	assert.False(t, exists)
	assert.Nil(t, template)
}

func TestBuildIssue(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{
		id:       "test-issue",
		template: &mockIssueTemplate{issueID: "test-issue"},
	})

	issue, err := registry.BuildIssue("test-issue", map[string]string{"key": "test-value"})

	require.NoError(t, err)
	assert.Equal(t, "test-issue", issue.Id)
	assert.Equal(t, "Test Issue: test-issue", issue.Title)
	assert.Equal(t, "Context value: test-value", issue.Description)
}

func TestBuildIssueNotFound(t *testing.T) {
	registry := NewRegistry()

	issue, err := registry.BuildIssue("non-existent-issue", nil)

	assert.Error(t, err)
	assert.Nil(t, issue)
	assert.Contains(t, err.Error(), "no issue template found for: non-existent-issue")
}

func TestGetBuiltInChecksReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{
		id:       "test-issue",
		template: &mockIssueTemplate{issueID: "test-issue"},
	})

	checks1 := registry.GetBuiltInChecks()
	checks2 := registry.GetBuiltInChecks()

	// Verify they are different slices (copies)
	assert.NotSame(t, &checks1[0], &checks2[0])

	// Modify one and verify it doesn't affect the other
	checks1[0] = nil
	assert.NotNil(t, checks2[0])
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	var wg sync.WaitGroup

	// Concurrent registrations
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			module := &mockModuleWithCheck{
				id:       "concurrent-issue-" + string(rune('A'+idx)),
				template: &mockIssueTemplate{issueID: "concurrent-issue-" + string(rune('A'+idx))},
			}
			registry.RegisterModule(module)
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.GetBuiltInChecks()
			registry.GetTemplate("concurrent-issue-A")
		}()
	}

	wg.Wait()

	// Verify all registrations completed
	checks := registry.GetBuiltInChecks()
	assert.Len(t, checks, 10)
}
