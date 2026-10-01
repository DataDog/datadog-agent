// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a controllable cluster ID resolver for component tests.
package mock

import (
	"context"
	"sync"

	clusteridresolver "github.com/DataDog/datadog-agent/comp/core/clusteridresolver/def"
)

// Mock represents a pending or resolved cluster identity.
type Mock struct {
	mu    sync.RWMutex
	id    string
	err   error
	ready chan struct{}
}

var _ clusteridresolver.Component = (*Mock)(nil)

// New creates a resolver whose ID has not yet been resolved.
func New() *Mock { return &Mock{err: clusteridresolver.ErrNotResolved, ready: make(chan struct{})} }

// NewResolved creates a resolver whose ID is already resolved.
func NewResolved(id string) *Mock {
	m := New()
	m.SetID(id)
	return m
}

// SetID resolves the mock's ID and releases its waiters.
func (m *Mock) SetID(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.id, m.err = id, nil
	select {
	case <-m.ready:
	default:
		close(m.ready)
	}
}

// GetID reads the current state without waiting.
func (m *Mock) GetID() (string, error) { m.mu.RLock(); defer m.mu.RUnlock(); return m.id, m.err }

// WaitForID waits for SetID or cancellation.
func (m *Mock) WaitForID(ctx context.Context) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-m.ready:
		return m.GetID()
	}
}
