// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package runnerimpl

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	registrymock "github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/mock"
	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	storemock "github.com/DataDog/datadog-agent/comp/healthplatform/store/mock"
)

// failingTemplate is a Template whose BuildIssue always errors, used to
// exercise runner.toProto's minimal-fallback path.
type failingTemplate struct {
	issueName string
	issueType string
}

func (f *failingTemplate) IssueName() string { return f.issueName }
func (f *failingTemplate) IssueType() string { return f.issueType }
func (f *failingTemplate) BuildIssue(_ map[string]string) (*healthplatformpayload.Issue, error) {
	return nil, errors.New("build failed")
}

func newTestRunner(t *testing.T) (*runner, *storemock.Mock) {
	t.Helper()
	store := storemock.New(t)
	r := &runner{
		log:      logmock.New(t),
		registry: registrymock.New(t),
		store:    store,
	}
	return r, store
}

func TestRunHappyPath(t *testing.T) {
	r, store := newTestRunner(t)

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a", Source: "mycomp"},
			{IssueID: "id-2", IssueName: "type-b", Source: "mycomp"},
		}, nil
	}

	ids, err := r.Run("mycomp", fn)
	require.NoError(t, err)
	assert.Equal(t, []string{"id-1", "id-2"}, ids)
	count, _ := store.GetAllIssues()
	assert.Equal(t, 2, count)
}

func TestRunEmptyResult(t *testing.T) {
	r, store := newTestRunner(t)

	ids, err := r.Run("mycomp", func() ([]runnerdef.IssueReport, error) {
		return nil, nil
	})

	require.NoError(t, err)
	assert.Empty(t, ids)
	count, _ := store.GetAllIssues()
	assert.Equal(t, 0, count)
}

func TestRunFnError(t *testing.T) {
	r, store := newTestRunner(t)

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a", Source: "mycomp"},
		}, errors.New("probe failed")
	}

	ids, err := r.Run("mycomp", fn)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "probe failed")
	// Report emitted before error is still forwarded.
	assert.Equal(t, []string{"id-1"}, ids)
	count, _ := store.GetAllIssues()
	assert.Equal(t, 1, count)
}

func TestRunFnPanic(t *testing.T) {
	r, store := newTestRunner(t)

	ids, err := r.Run("mycomp", func() ([]runnerdef.IssueReport, error) {
		panic("something exploded")
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "panic")
	assert.Empty(t, ids)
	count, _ := store.GetAllIssues()
	assert.Equal(t, 0, count)
}

func TestRunSourceDefaultFill(t *testing.T) {
	r, store := newTestRunner(t)

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a"}, // Source intentionally empty
		}, nil
	}

	_, err := r.Run("fallback-source", fn)
	require.NoError(t, err)
	issue := store.GetIssue("id-1")
	require.NotNil(t, issue)
	// The mock registry has no template for "type-a" so a minimal proto is built
	// with the source filled from the fallback.
	assert.Equal(t, "fallback-source", issue.Source)
}

// TestRunFallbackUsesTemplateIssueType guards that when a template is found by
// the registry but BuildIssue errors, the minimal fallback proto still carries
// the template's own IssueType instead of leaving it empty.
func TestRunFallbackUsesTemplateIssueType(t *testing.T) {
	tmpl := &failingTemplate{issueName: "type-a", issueType: "type_a_snake"}
	store := storemock.New(t)
	r := &runner{
		log:      logmock.New(t),
		registry: registrymock.New(t, registrymock.WithTemplate("type-a", tmpl)),
		store:    store,
	}

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a", Source: "mycomp"},
		}, nil
	}

	_, err := r.Run("mycomp", fn)
	require.NoError(t, err)

	issue := store.GetIssue("id-1")
	require.NotNil(t, issue)
	assert.Equal(t, "type_a_snake", issue.IssueType)
}

func TestRunSourceNotOverridden(t *testing.T) {
	r, store := newTestRunner(t)

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a", Source: "explicit-source"},
		}, nil
	}

	_, err := r.Run("fallback-source", fn)
	require.NoError(t, err)
	issue := store.GetIssue("id-1")
	require.NotNil(t, issue)
	assert.Equal(t, "explicit-source", issue.Source)
}

func TestRunStoreError(t *testing.T) {
	store := storemock.New(t, storemock.WithReportIssueError("id-2", errors.New("store rejected")))
	r := &runner{
		log:      logmock.New(t),
		registry: registrymock.New(t),
		store:    store,
	}

	fn := func() ([]runnerdef.IssueReport, error) {
		return []runnerdef.IssueReport{
			{IssueID: "id-1", IssueName: "type-a", Source: "mycomp"},
			{IssueID: "id-2", IssueName: "type-b", Source: "mycomp"},
		}, nil
	}

	ids, err := r.Run("mycomp", fn)
	require.NoError(t, err)
	// id-1 accepted, id-2 rejected by store — only id-1 in returned slice.
	assert.Equal(t, []string{"id-1"}, ids)
	assert.NotNil(t, store.GetIssue("id-1"))
	assert.Nil(t, store.GetIssue("id-2"))
}

func TestRunConcurrent(t *testing.T) {
	r, store := newTestRunner(t)

	var wg sync.WaitGroup
	const n = 10
	callCount := int32(0)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			fn := func() ([]runnerdef.IssueReport, error) {
				atomic.AddInt32(&callCount, 1)
				return []runnerdef.IssueReport{
					{IssueID: fmt.Sprintf("id-%d", idx), IssueName: "type-a", Source: "mycomp"},
				}, nil
			}
			r.Run("mycomp", fn) //nolint:errcheck
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(n), atomic.LoadInt32(&callCount))
	count, _ := store.GetAllIssues()
	assert.Equal(t, n, count)
}

// Ensure HealthCheckFunc type is used correctly in tests.
var _ runnerdef.HealthCheckFunc = func() ([]runnerdef.IssueReport, error) { return nil, nil }
