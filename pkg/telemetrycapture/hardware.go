// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"context"
	"slices"
)

type hostSystemInfoRequest struct {
	done    chan struct{}
	context context.Context
	cancel  context.CancelFunc
	err     error
}

// SetHostSystemInfoCollector registers the only capture-triggered collector.
// Registration does not advertise readiness. The provider must first complete
// ordinary collection and must check context/session validity before submitting.
// A nil callback removes it during provider shutdown.
func (m *Manager) SetHostSystemInfoCollector(collect func(context.Context, Control) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.hostSystemInfoCollector = collect
}

// RequestHostSystemInfo requests one fresh normal hardware submission per
// session. Repeated requests wait for that same operation; no generic check
// execution or collection schedule mutation is exposed.
func (m *Manager) RequestHostSystemInfo(ctx context.Context, control Control) (Status, error) {
	m.mu.Lock()
	s, err := m.sessionLocked(control)
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	if s.state != Active || m.identity.Role != "core-agent" || !slices.Contains(s.streams, HostSystemInfo) ||
		m.hostSystemInfoCollector == nil || !slices.ContainsFunc(m.capabilities, func(c Capability) bool { return c.Stream == HostSystemInfo }) {
		m.mu.Unlock()
		return Status{}, ErrState
	}
	if s.hostSystemInfo == nil {
		if m.hostSystemInfoRunning {
			m.mu.Unlock()
			return Status{}, ErrBusy
		}
		if ctx.Err() != nil {
			m.mu.Unlock()
			return Status{}, ErrFailed
		}
		work, cancel := context.WithCancel(ctx)
		s.hostSystemInfo = &hostSystemInfoRequest{done: make(chan struct{}), context: work, cancel: cancel}
		m.hostSystemInfoRunning = true
		go m.collectHostSystemInfo(work, control, s, m.hostSystemInfoCollector)
	}
	request := s.hostSystemInfo
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		_ = m.Fail(control)
		return Status{}, ErrFailed
	case <-request.context.Done():
		select {
		case <-request.done: // A completed request releases its context too.
		default:
			m.mu.Lock()
			if s.state == Active {
				m.failLocked(s, false)
			}
			m.mu.Unlock()
			return Status{}, ErrFailed
		}
	case <-request.done:
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	current, err := m.sessionLocked(control)
	if err != nil {
		return Status{}, err
	}
	if request.err != nil || current.state == Failed {
		return Status{}, ErrFailed
	}
	if current.state != Active {
		return Status{}, ErrState
	}
	return m.statusForLocked(current), nil
}

func (m *Manager) collectHostSystemInfo(ctx context.Context, control Control, s *session, collect func(context.Context, Control) error) {
	var err error
	defer func() {
		if recover() != nil {
			err = ErrFailed
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		m.hostSystemInfoRunning = false
		if err != nil {
			s.hostSystemInfo.err = ErrFailed
			if s.state == Active {
				m.failLocked(s, false)
			}
		}
		close(s.hostSystemInfo.done)
		s.hostSystemInfo.cancel()
	}()
	err = collect(ctx, control)
}
