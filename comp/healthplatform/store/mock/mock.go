// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock implementation of the health-platform component
package mock

import (
	"sync"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

type Mock struct {
	t             testing.TB
	mu            sync.Mutex
	issues        map[string]*healthplatformpayload.Issue
	observer      healthplatform.IssuesObserver
	resolvedIDs   []string
	reportErrOnID string
	reportErr     error
}

// Option configures the mock store returned by New.
type Option func(*Mock)

// WithIssue pre-populates the store with an issue.
func WithIssue(issue *healthplatformpayload.Issue) Option {
	return func(m *Mock) {
		m.issues[issue.Id] = proto.Clone(issue).(*healthplatformpayload.Issue)
	}
}

// WithReportIssueError makes ReportIssue return err for issueID; other issues
// are stored normally.
func WithReportIssueError(issueID string, err error) Option {
	return func(m *Mock) {
		m.reportErrOnID = issueID
		m.reportErr = err
	}
}

// New returns a mock health platform store for testing.
func New(t testing.TB, opts ...Option) *Mock {
	m := &Mock{t: t, issues: make(map[string]*healthplatformpayload.Issue)}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Observer returns the observer registered via RegisterIssuesObserver.
func (m *Mock) Observer() healthplatform.IssuesObserver {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.observer
}

// ResolvedIDs returns the IDs passed to ResolveIssue, in call order.
func (m *Mock) ResolvedIDs() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.resolvedIDs))
	copy(out, m.resolvedIDs)
	return out
}

func (m *Mock) RegisterIssuesObserver(obs healthplatform.IssuesObserver) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.observer = obs
}

func (m *Mock) ReportIssue(issue *healthplatformpayload.Issue) error {
	m.t.Helper()
	if issue == nil || issue.Id == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reportErr != nil && issue.Id == m.reportErrOnID {
		return m.reportErr
	}
	m.issues[issue.Id] = proto.Clone(issue).(*healthplatformpayload.Issue)
	return nil
}

func (m *Mock) GetAllIssues() (int, map[string]*healthplatformpayload.Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	result := make(map[string]*healthplatformpayload.Issue)
	for id, issue := range m.issues {
		if issue != nil {
			result[id] = proto.Clone(issue).(*healthplatformpayload.Issue)
			count++
		}
	}
	return count, result
}

func (m *Mock) GetIssue(issueID string) *healthplatformpayload.Issue {
	m.mu.Lock()
	defer m.mu.Unlock()
	issue := m.issues[issueID]
	if issue == nil {
		return nil
	}
	return proto.Clone(issue).(*healthplatformpayload.Issue)
}

func (m *Mock) ResolveIssue(issueID string) {
	m.mu.Lock()
	issue := m.issues[issueID]
	delete(m.issues, issueID)
	m.resolvedIDs = append(m.resolvedIDs, issueID)
	obs := m.observer
	m.mu.Unlock()
	if issue == nil {
		return
	}
	notifyResolved(obs, issue)
}

func (m *Mock) ResolveAllIssues() {
	m.mu.Lock()
	issues := m.issues
	m.issues = make(map[string]*healthplatformpayload.Issue)
	obs := m.observer
	m.mu.Unlock()
	// Mirrors the real store: ResolveAllIssues notifies observers with a
	// resolved tombstone for every issue that was still active, the same as
	// resolving each of them individually.
	for _, issue := range issues {
		if issue != nil {
			notifyResolved(obs, issue)
		}
	}
}

// notifyResolved sends a resolved tombstone for issue to obs, mirroring the
// real store: ResolveIssue/ResolveAllIssues always notify with
// State == RESOLVED, regardless of the issue's state beforehand, and the
// send is non-blocking so a full or unread observer channel can't hang the
// caller.
func notifyResolved(obs healthplatform.IssuesObserver, issue *healthplatformpayload.Issue) {
	if obs.ResolvedCh == nil {
		return
	}
	resolved := proto.Clone(issue).(*healthplatformpayload.Issue)
	if resolved.PersistedIssue == nil {
		resolved.PersistedIssue = &healthplatformpayload.PersistedIssue{}
	}
	resolved.PersistedIssue.State = healthplatformpayload.IssueState_ISSUE_STATE_RESOLVED
	select {
	case obs.ResolvedCh <- resolved:
	default:
	}
}

func (m *Mock) GetActiveIssueIDsByIssueName(issueName string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var ids []string
	for id, issue := range m.issues {
		if issue != nil && issue.IssueName == issueName {
			ids = append(ids, id)
		}
	}
	return ids
}

// IssueDiscriminator returns hostID unchanged; tests that care about
// deployment_id collapse should assert against selfident directly.
func (m *Mock) IssueDiscriminator(hostID string) string {
	return hostID
}
