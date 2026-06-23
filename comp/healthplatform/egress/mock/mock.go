// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform egress component.
package mock

import (
	"context"
	"sync"
	"testing"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"
	"google.golang.org/protobuf/proto"

	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

const resolvedChBuf = 64

// Mock is a test implementation of egressdef.Component.
// It mirrors the real egress's tick logic (store→forwarder merge with resolved
// tombstone handling) so tests can drive forwarding deterministically via Tick()
// instead of relying on background goroutines or timing.
//
// New registers itself as a store observer so that ResolveIssue calls on the
// store are automatically funnelled into the resolved tombstone map, exactly
// as the real egress does.
type Mock struct {
	t          testing.TB
	store      storedef.Component
	forwarder  forwarderdef.Component
	resolvedCh chan *healthplatformpayload.Issue
	mu         sync.Mutex
	resolved   map[string]*healthplatformpayload.Issue
}

// New returns a mock egress for testing.
// It registers with store as an issues observer so resolved tombstones flow
// through automatically, mirroring the real egress dependency graph.
func New(t testing.TB, store storedef.Component, forwarder forwarderdef.Component) *Mock { //nolint:revive
	m := &Mock{
		t:          t,
		store:      store,
		forwarder:  forwarder,
		resolvedCh: make(chan *healthplatformpayload.Issue, resolvedChBuf),
		resolved:   make(map[string]*healthplatformpayload.Issue),
	}
	store.RegisterIssuesObserver(storedef.IssuesObserver{ResolvedCh: m.resolvedCh})
	return m
}

// Tick simulates one egress flush: drains resolved tombstones from the store
// observer channel, merges them with active issues (active wins on conflict),
// calls forwarder.Send, and clears tombstones on success — mirroring the real
// egress tick() without background goroutines.
func (m *Mock) Tick(ctx context.Context) error {
	m.t.Helper()

	// Drain resolved notifications that arrived since the last Tick.
	m.mu.Lock()
	for {
		select {
		case issue := <-m.resolvedCh:
			m.resolved[issue.Id] = proto.Clone(issue).(*healthplatformpayload.Issue)
		default:
			goto drained
		}
	}
drained:
	m.mu.Unlock()

	count, active := m.store.GetAllIssues()

	m.mu.Lock()
	resolvedCount := len(m.resolved)
	if count == 0 && resolvedCount == 0 {
		m.mu.Unlock()
		return nil
	}

	// Merge: resolved first, then active overwrites (active wins on recurrence).
	merged := make(map[string]*healthplatformpayload.Issue, count+resolvedCount)
	for id, issue := range m.resolved {
		merged[id] = issue
	}
	m.mu.Unlock()

	for id, issue := range active {
		merged[id] = issue
	}

	report := &healthplatformpayload.HealthReport{Issues: merged}
	if err := m.forwarder.Send(ctx, report); err != nil {
		return err
	}

	m.mu.Lock()
	m.resolved = make(map[string]*healthplatformpayload.Issue)
	m.mu.Unlock()
	return nil
}

// Resolved returns a snapshot of the current resolved-tombstone map.
func (m *Mock) Resolved() map[string]*healthplatformpayload.Issue {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[string]*healthplatformpayload.Issue, len(m.resolved))
	for k, v := range m.resolved {
		out[k] = proto.Clone(v).(*healthplatformpayload.Issue)
	}
	return out
}

// AddResolved pre-populates a resolved tombstone, equivalent to what the real
// egress accumulates from store ResolveIssue calls between ticks.
func (m *Mock) AddResolved(issue *healthplatformpayload.Issue) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resolved[issue.Id] = proto.Clone(issue).(*healthplatformpayload.Issue)
}
