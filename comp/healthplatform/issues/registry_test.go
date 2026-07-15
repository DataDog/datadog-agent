// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package issues

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/healthplatform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
)

// mockModuleWithCheck has a periodic check only.
type mockModuleWithCheck struct {
	id string
}

func (m *mockModuleWithCheck) IssueName() string { return m.id }
func (m *mockModuleWithCheck) IssueType() string { return m.id }
func (m *mockModuleWithCheck) BuildIssue(context map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Title:       "Test Issue: " + m.id,
		Description: "Context value: " + context["key"],
		Severity:    healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
	}, nil
}
func (m *mockModuleWithCheck) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return &runnerdef.BuiltInPeriodicHealthCheck{
		BuiltInHealthCheck: runnerdef.BuiltInHealthCheck{
			Source: "check-" + m.id,
			Fn:     func() ([]runnerdef.IssueReport, error) { return nil, nil },
		},
		Interval: 5 * time.Minute,
	}
}
func (m *mockModuleWithCheck) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

// mockModuleWithOnce has a once check only.
type mockModuleWithOnce struct {
	id string
}

func (m *mockModuleWithOnce) IssueName() string { return m.id }
func (m *mockModuleWithOnce) IssueType() string { return m.id }
func (m *mockModuleWithOnce) BuildIssue(_ map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Title:    "Test Issue: " + m.id,
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
	}, nil
}
func (m *mockModuleWithOnce) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}
func (m *mockModuleWithOnce) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return &runnerdef.BuiltInHealthCheck{
		Source: "once-" + m.id,
		Fn:     func() ([]runnerdef.IssueReport, error) { return nil, nil },
	}
}

// mockModuleWithoutCheck has neither check type.
type mockModuleWithoutCheck struct {
	id string
}

func (m *mockModuleWithoutCheck) IssueName() string { return m.id }
func (m *mockModuleWithoutCheck) IssueType() string { return m.id }
func (m *mockModuleWithoutCheck) BuildIssue(_ map[string]string) (*healthplatform.Issue, error) {
	return &healthplatform.Issue{
		Title:    "Test Issue: " + m.id,
		Severity: healthplatform.IssueSeverity_ISSUE_SEVERITY_MEDIUM,
	}, nil
}
func (m *mockModuleWithoutCheck) BuiltInPeriodicHealthCheck() *runnerdef.BuiltInPeriodicHealthCheck {
	return nil
}
func (m *mockModuleWithoutCheck) BuiltInStartupHealthCheck() *runnerdef.BuiltInHealthCheck {
	return nil
}

func TestNewRegistry(t *testing.T) {
	registry := NewRegistry()
	assert.NotNil(t, registry)
	assert.Empty(t, registry.templates)
	assert.Empty(t, registry.periodicChecks)
	assert.Empty(t, registry.healthChecks)
}

func TestRegisterModuleWithPeriodicCheck(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{id: "Test Issue One"})

	_, exists := registry.GetTemplate("Test Issue One")
	assert.True(t, exists)

	checks := registry.GetBuiltInPeriodicHealthChecks()
	assert.Len(t, checks, 1)
	assert.Equal(t, "check-Test Issue One", checks[0].Source)
	assert.Equal(t, 5*time.Minute, checks[0].Interval)

	assert.Empty(t, registry.GetBuiltInStartupHealthChecks())
}

func TestRegisterModuleWithOnceCheck(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithOnce{id: "Test Issue Two"})

	_, exists := registry.GetTemplate("Test Issue Two")
	assert.True(t, exists)

	once := registry.GetBuiltInStartupHealthChecks()
	assert.Len(t, once, 1)
	assert.Equal(t, "once-Test Issue Two", once[0].Source)

	assert.Empty(t, registry.GetBuiltInPeriodicHealthChecks())
}

func TestRegisterModuleWithoutCheck(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithoutCheck{id: "Test Issue Three"})

	_, exists := registry.GetTemplate("Test Issue Three")
	assert.True(t, exists)
	assert.Empty(t, registry.GetBuiltInPeriodicHealthChecks())
	assert.Empty(t, registry.GetBuiltInStartupHealthChecks())
}

func TestRegisterMultipleModules(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{id: "Periodic Check"})
	registry.RegisterModule(&mockModuleWithOnce{id: "Once Check"})
	registry.RegisterModule(&mockModuleWithoutCheck{id: "Neither Check"})

	assert.Len(t, registry.GetBuiltInPeriodicHealthChecks(), 1)
	assert.Len(t, registry.GetBuiltInStartupHealthChecks(), 1)
}

func TestGetTemplateNotFound(t *testing.T) {
	registry := NewRegistry()
	template, exists := registry.GetTemplate("non-existent-issue")
	assert.False(t, exists)
	assert.Nil(t, template)
}

func TestBuildIssue(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{id: "Test Issue"})

	issue, err := registry.BuildIssue("Test Issue", map[string]string{"key": "test-value"})
	require.NoError(t, err)
	assert.Empty(t, issue.Id, "Id is set by the caller (ReportIssue), not by the template")
	assert.Equal(t, "Test Issue: Test Issue", issue.Title)
	assert.Equal(t, "Context value: test-value", issue.Description)
}

func TestBuildIssueNotFound(t *testing.T) {
	registry := NewRegistry()
	issue, err := registry.BuildIssue("Nonexistent Issue", nil)
	assert.Error(t, err)
	assert.Nil(t, issue)
	assert.Contains(t, err.Error(), "no issue template found for: Nonexistent Issue")
}

func TestGetBuiltInPeriodicHealthChecksReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithCheck{id: "Test Issue"})

	checks1 := registry.GetBuiltInPeriodicHealthChecks()
	checks2 := registry.GetBuiltInPeriodicHealthChecks()
	assert.NotSame(t, &checks1[0], &checks2[0])
	checks1[0] = nil
	assert.NotNil(t, checks2[0])
}

func TestGetBuiltInStartupHealthChecksReturnsCopy(t *testing.T) {
	registry := NewRegistry()
	registry.RegisterModule(&mockModuleWithOnce{id: "Test Issue Once"})

	once1 := registry.GetBuiltInStartupHealthChecks()
	once2 := registry.GetBuiltInStartupHealthChecks()
	assert.NotSame(t, &once1[0], &once2[0])
	once1[0] = nil
	assert.NotNil(t, once2[0])
}

func TestRegistryConcurrentAccess(t *testing.T) {
	registry := NewRegistry()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			registry.RegisterModule(&mockModuleWithCheck{
				id: fmt.Sprintf("Concurrent Issue %d", idx),
			})
		}(i)
	}

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.GetBuiltInPeriodicHealthChecks()
			registry.GetBuiltInStartupHealthChecks()
			registry.GetTemplate("Concurrent Issue 0")
		}()
	}

	wg.Wait()
	assert.Len(t, registry.GetBuiltInPeriodicHealthChecks(), 10)
}
