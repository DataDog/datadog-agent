// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetrycapture

import (
	"context"
	"crypto/rand"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// Fixed errors must not include telemetry or request bodies.
var (
	ErrProtocol = errors.New("unsupported capture protocol")
	ErrRequest  = errors.New("invalid capture request")
	ErrSession  = errors.New("capture session mismatch")
	ErrBusy     = errors.New("capture producer busy")
	ErrState    = errors.New("invalid capture state")
	ErrCursor   = errors.New("invalid capture cursor")
	ErrFailed   = errors.New("capture session failed")
	ErrClosed   = errors.New("capture producer closed")
)

type session struct {
	id             string
	streams        []Stream
	state          State
	lease          time.Time
	activatedAt    time.Time
	stoppedAt      time.Time
	sequence       uint64
	ack            uint64
	delivered      uint64
	failures       uint64
	drops          uint64
	bytes          int64
	records        int
	pending        int
	reading        bool
	queue          []*entry
	changed        chan struct{}
	hostSystemInfo *hostSystemInfoRequest
}

// Manager belongs to one daemon. Its tees and authenticated API must share the
// same instance. It must be closed by that daemon's lifecycle, not by adapters.
// Locks protect only bounded bookkeeping; copying, encoding, and I/O must never
// happen while they are held.
type Manager struct {
	active                  atomic.Pointer[session]
	mu                      sync.Mutex
	identity                Identity
	capabilities            []Capability
	current                 *session
	cycle                   uint64
	closed                  bool
	shutdown                chan struct{}
	done                    chan struct{}
	hostSystemInfoCollector func(context.Context, Control) error
	hostSystemInfoRunning   bool
}

// NewManager creates a dormant process-local manager with a fresh process
// instance ID. Construct it at daemon startup, never per serializer or sender.
func NewManager(role, version, commit string) *Manager {
	m := &Manager{
		identity: Identity{Role: role, Version: version, Commit: commit, InstanceID: rand.Text()},
		shutdown: make(chan struct{}),
		done:     make(chan struct{}),
	}
	go m.reap()
	return m
}

func (m *Manager) reap() {
	defer close(m.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.shutdown:
			return
		case <-ticker.C:
			m.mu.Lock()
			m.expireLocked()
			m.mu.Unlock()
		}
	}
}

// Enabled is the entire disabled fast path; it does not allocate or lock.
func (m *Manager) Enabled() bool { return m != nil && m.active.Load() != nil }

// Selected returns the immutable identity of the observed active session. An
// adapter that invokes external projection code before reserving must bind both
// failures and its eventual reservation to this identity.
func (m *Manager) Selected(stream Stream) (Control, bool) {
	if m == nil {
		return Control{}, false
	}
	s := m.active.Load()
	if s == nil || !slices.Contains(s.streams, stream) {
		return Control{}, false
	}
	return Control{ProtocolVersion: ProtocolVersion, SessionID: s.id}, true
}

// Register advertises a running producer, including its effective schedule.
// New streams or ownership changes invalidate a session; normal cadence changes
// update readiness while each accepted record retains its own effective cadence.
func (m *Manager) Register(capability Capability) error {
	if !validStream(capability.Stream) || capability.Cadence <= 0 || len(capability.ConnectionOwner) > 64 ||
		len(capability.MetricSchedules) > 7 || (capability.Stream != Metrics && len(capability.MetricSchedules) != 0) {
		return ErrRequest
	}
	families := make(map[string]bool, len(capability.MetricSchedules))
	for _, schedule := range capability.MetricSchedules {
		if MetricCheckFamily(schedule.Family) == "" || schedule.Cadence <= 0 || families[schedule.Family] {
			return ErrRequest
		}
		families[schedule.Family] = true
	}
	capability.MetricSchedules = slices.Clone(capability.MetricSchedules)
	for i := range capability.MetricSchedules {
		capability.MetricSchedules[i].Family = MetricCheckFamily(capability.MetricSchedules[i].Family)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	for i, existing := range m.capabilities {
		if existing.Stream == capability.Stream {
			if existing.ConnectionOwner != capability.ConnectionOwner {
				m.failLocked(m.current, false)
			}
			m.capabilities[i] = capability
			return nil
		}
	}
	m.failLocked(m.current, false)
	m.capabilities = append(m.capabilities, capability)
	slices.SortFunc(m.capabilities, func(a, b Capability) int {
		if a.Stream < b.Stream {
			return -1
		}
		if a.Stream > b.Stream {
			return 1
		}
		return 0
	})
	return nil
}

// Unregister disarms capture when a participating producer stops.
func (m *Manager) Unregister(stream Stream) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, capability := range m.capabilities {
		if capability.Stream == stream {
			m.capabilities = slices.Delete(m.capabilities, i, i+1)
			if s := m.current; s != nil && slices.Contains(s.streams, stream) {
				m.failLocked(s, false)
			}
			return
		}
	}
}

// Prepare reserves this producer for one coordinator without arming its tees.
func (m *Manager) Prepare(request PrepareRequest) (Status, error) {
	if err := validateControl(request.Control); err != nil {
		return Status{}, err
	}
	if len(request.Streams) == 0 || len(request.Streams) > 8 {
		return Status{}, ErrRequest
	}
	streams := slices.Clone(request.Streams)
	slices.Sort(streams)
	for i, stream := range streams {
		if !validStream(stream) || (i > 0 && streams[i-1] == stream) {
			return Status{}, ErrRequest
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	if m.closed {
		return Status{}, ErrClosed
	}
	if s := m.current; s != nil {
		if s.id == request.SessionID {
			if !slices.Equal(s.streams, streams) {
				return Status{}, ErrRequest
			}
			return m.statusLocked(), nil
		}
		if (s.state != Stopped && s.state != Failed) || s.records != 0 || s.bytes != 0 || s.reading {
			return Status{}, ErrBusy
		}
	}
	// A cancelled hardware collector may still be returning from native code.
	// Do not let its normal serializer tee observe a newer session.
	if m.hostSystemInfoRunning {
		return Status{}, ErrBusy
	}
	for _, stream := range streams {
		if !slices.ContainsFunc(m.capabilities, func(c Capability) bool { return c.Stream == stream }) {
			return Status{}, ErrState
		}
	}
	m.current = &session{
		id: request.SessionID, streams: streams, state: Prepared,
		lease: time.Now().Add(LeaseDuration), changed: make(chan struct{}),
	}
	return m.statusLocked(), nil
}

// Activate arms a prepared session exactly once and returns its actual boundary.
func (m *Manager) Activate(control Control) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sessionLocked(control)
	if err != nil {
		return Status{}, err
	}
	if s.state == Prepared {
		s.activatedAt = time.Now()
		s.lease = s.activatedAt.Add(LeaseDuration)
		s.state = Active
		m.active.Store(s)
	}
	return m.statusLocked(), nil
}

// Heartbeat renews a prepared, active, or draining session's lease.
func (m *Manager) Heartbeat(control Control) (Status, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sessionLocked(control)
	if err != nil {
		return Status{}, err
	}
	if s.state != Stopped && s.state != Failed {
		s.lease = time.Now().Add(LeaseDuration)
	}
	return m.statusLocked(), nil
}

// Stop atomically closes admission and waits for previously admitted copies.
// It returns Stopping until records have been read and the final sequence has
// been acknowledged. Repeating Stop never admits new observations.
func (m *Manager) Stop(ctx context.Context, control Control) (Status, error) {
	m.mu.Lock()
	s, err := m.sessionLocked(control)
	if err != nil {
		m.mu.Unlock()
		return Status{}, err
	}
	if s.state == Prepared || s.state == Active {
		m.active.Store(nil)
		s.state = Stopping
		s.stoppedAt = time.Now()
		if s.hostSystemInfo != nil {
			s.hostSystemInfo.cancel()
		}
		m.notifyLocked(s)
	}
	for s.pending != 0 && s.state == Stopping {
		changed := s.changed
		m.mu.Unlock()
		select {
		case <-ctx.Done():
			return Status{}, ctx.Err()
		case <-changed:
		}
		m.mu.Lock()
	}
	m.finishLocked(s)
	status := m.statusForLocked(s)
	m.mu.Unlock()
	return status, nil
}

// Fail invalidates only the matching session, for example on worker failure.
// Failure reasons are intentionally not accepted from telemetry-processing code.
func (m *Manager) Fail(control Control) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, err := m.sessionLocked(control)
	if err != nil {
		return err
	}
	m.failLocked(s, false)
	return nil
}

// Status returns no queue contents, credentials, or telemetry.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.expireLocked()
	return m.statusLocked()
}

// Close disarms capture and releases retained records during daemon shutdown.
// In-progress copies and pinned reads release their reservations on completion.
func (m *Manager) Close() {
	m.mu.Lock()
	if !m.closed {
		m.closed = true
		m.failLocked(m.current, false)
		m.capabilities = nil
		close(m.shutdown)
	}
	m.mu.Unlock()
	<-m.done
}

func validateControl(control Control) error {
	if control.ProtocolVersion != ProtocolVersion {
		return ErrProtocol
	}
	if len(control.SessionID) < 16 || len(control.SessionID) > 64 {
		return ErrRequest
	}
	for _, c := range control.SessionID {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '-' && c != '_' {
			return ErrRequest
		}
	}
	return nil
}

func validStream(stream Stream) bool {
	return stream == Metrics || stream == Metadata || stream == Processes || stream == Connections || stream == Software || stream == AgentInventory || stream == HostInventory || stream == HostSystemInfo
}

func (m *Manager) sessionLocked(control Control) (*session, error) {
	if err := validateControl(control); err != nil {
		return nil, err
	}
	m.expireLocked()
	if m.closed {
		return nil, ErrClosed
	}
	if m.current == nil || m.current.id != control.SessionID {
		return nil, ErrSession
	}
	return m.current, nil
}

func (m *Manager) expireLocked() {
	if s := m.current; s != nil && s.state != Failed && s.state != Stopped && !time.Now().Before(s.lease) {
		m.failLocked(s, false)
	}
}

func (m *Manager) notifyLocked(s *session) {
	close(s.changed)
	s.changed = make(chan struct{})
}

func (m *Manager) failLocked(s *session, dropped bool) {
	if s == nil || s.state == Failed || s.state == Stopped {
		return
	}
	m.active.CompareAndSwap(s, nil)
	s.state = Failed
	if s.hostSystemInfo != nil {
		s.hostSystemInfo.cancel()
	}
	s.failures++
	if dropped {
		s.drops++
	}
	for _, e := range s.queue {
		e.retired = true
		m.releaseLocked(s, e)
	}
	s.queue = nil
	m.notifyLocked(s)
}

func (m *Manager) finishLocked(s *session) {
	if s.state == Stopping && s.pending == 0 && s.ack == s.sequence && s.records == 0 {
		s.state = Stopped
		m.notifyLocked(s)
	}
}

func (m *Manager) statusLocked() Status { return m.statusForLocked(m.current) }

func (m *Manager) statusForLocked(s *session) Status {
	status := Status{
		ProtocolVersion: ProtocolVersion, Producer: m.identity,
		Capabilities: slices.Clone(m.capabilities),
	}
	for i := range status.Capabilities {
		status.Capabilities[i].MetricSchedules = slices.Clone(status.Capabilities[i].MetricSchedules)
	}
	if s != nil {
		status.SessionID, status.State = s.id, s.state
		status.ActivatedAt, status.StoppedAt = s.activatedAt, s.stoppedAt
		status.FinalSequence, status.Acknowledged = s.sequence, s.ack
		status.Failures, status.Drops = s.failures, s.drops
	}
	return status
}
