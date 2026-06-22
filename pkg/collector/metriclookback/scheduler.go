// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	"github.com/DataDog/datadog-agent/pkg/collector/runner/expvars"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	defaultShadowInterval = time.Second
)

var errShadowSchedulerStopped = errors.New("shadow scheduler is stopped")

// SenderManagerFactory returns a sender manager scoped to one shadow check.
type SenderManagerFactory func(context.Context) sender.SenderManager

// CheckInstanceLoader loads a fresh check instance using the provided sender
// manager and normal collector loader-selection semantics.
type CheckInstanceLoader interface {
	LoadInstance(sender.SenderManager, integration.Config, integration.Data, int) (check.Check, bool, error)
}

// CheckScheduler registers shadow checks with the collector scheduling path.
type CheckScheduler interface {
	RunCheck(check.Check) (checkid.ID, error)
	StopCheck(checkid.ID) error
}

// ShadowSchedulerOptions configures the shadow scheduler component.
type ShadowSchedulerOptions struct {
	Loader           CheckInstanceLoader
	NewSenderManager SenderManagerFactory
	Collector        CheckScheduler
	Interval         time.Duration
}

// ShadowScheduler reuses normal check-loading semantics through CheckInstanceLoader,
// but owns the shadow lifecycle policy: :shadow identity, lookback sender
// isolation, and cleanup of lookback-owned sender/stats state.
type ShadowScheduler struct {
	loader           CheckInstanceLoader
	newSenderManager SenderManagerFactory
	collector        CheckScheduler
	interval         time.Duration

	mu         sync.Mutex
	checks     map[shadowKey]*shadowCheckHandle
	checksByID map[checkid.ID]shadowKey
	stopped    bool
}

// NewShadowScheduler creates an isolated scheduler for lookback shadow checks.
func NewShadowScheduler(opts ShadowSchedulerOptions) *ShadowScheduler {
	interval := opts.Interval
	if interval == 0 {
		interval = defaultShadowInterval
	}
	return &ShadowScheduler{
		loader:           opts.Loader,
		newSenderManager: opts.NewSenderManager,
		collector:        opts.Collector,
		interval:         interval,
		checks:           make(map[shadowKey]*shadowCheckHandle),
		checksByID:       make(map[checkid.ID]shadowKey),
	}
}

// Schedule loads and starts shadow checks for the provided derived configs.
func (s *ShadowScheduler) Schedule(configs []ShadowConfig) error {
	if s.loader == nil {
		return errors.New("shadow scheduler loader is nil")
	}
	if s.newSenderManager == nil {
		return errors.New("shadow scheduler sender manager factory is nil")
	}
	if s.collector == nil {
		return errors.New("shadow scheduler collector is nil")
	}

	var firstErr error
	for _, config := range configs {
		key := newShadowKey(config)

		s.mu.Lock()
		if s.stopped {
			s.mu.Unlock()
			return errShadowSchedulerStopped
		}
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
			log.Warnf("shadow check %s is already scheduled", config.ShadowCheckID)
			continue
		}
		s.mu.Unlock()

		handle, err := s.load(config)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}

		s.mu.Lock()
		// LoadInstance is synchronous and does not accept a context, so re-check
		// scheduler state after loading to avoid late-starting after Stop.
		if s.stopped {
			s.mu.Unlock()
			handle.cleanup()
			if firstErr == nil {
				firstErr = errShadowSchedulerStopped
			}
			continue
		}
		if _, exists := s.checks[key]; exists {
			s.mu.Unlock()
			log.Warnf("shadow check %s was already scheduled while loading", config.ShadowCheckID)
			handle.cleanup()
			continue
		}
		s.checks[key] = handle
		s.checksByID[config.ShadowCheckID] = key
		if _, err := s.collector.RunCheck(handle.check); err != nil {
			delete(s.checks, key)
			delete(s.checksByID, config.ShadowCheckID)
			s.mu.Unlock()
			handle.cleanup()
			if firstErr == nil {
				firstErr = fmt.Errorf("schedule shadow check %s: %w", config.ShadowCheckID, err)
			}
			continue
		}
		s.mu.Unlock()
	}

	return firstErr
}

func (s *ShadowScheduler) load(config ShadowConfig) (*shadowCheckHandle, error) {
	ctx, cancel := context.WithCancel(context.Background())
	shadowSenderManager := &shadowSenderManager{
		delegate: s.newSenderManager(ctx),
		sourceID: config.SourceCheckID,
		shadowID: config.ShadowCheckID,
	}

	loadedCheck, loaded, err := s.loader.LoadInstance(shadowSenderManager, config.SourceConfig, config.Instance, config.InstanceIndex)
	if err != nil {
		shadowSenderManager.DestroySender(config.ShadowCheckID)
		cancel()
		return nil, fmt.Errorf("load shadow check %s: %w", config.ShadowCheckID, err)
	}
	if !loaded {
		shadowSenderManager.DestroySender(config.ShadowCheckID)
		cancel()
		return nil, fmt.Errorf("load shadow check %s: no check loaded", config.ShadowCheckID)
	}

	handle := &shadowCheckHandle{
		check:         &shadowCheck{Check: loadedCheck, id: config.ShadowCheckID, interval: s.interval},
		cancel:        cancel,
		senderManager: shadowSenderManager,
	}
	return handle, nil
}

// Unschedule stops and removes shadow checks matching the provided source
// configs. Waiting is bounded by the scheduler stop timeout.
func (s *ShadowScheduler) Unschedule(configs []ShadowConfig) error {
	var firstErr error
	for _, config := range configs {
		key := newShadowKey(config)

		s.mu.Lock()
		handle, found := s.checks[key]
		if found {
			delete(s.checks, key)
			delete(s.checksByID, config.ShadowCheckID)
		}
		s.mu.Unlock()

		if !found {
			continue
		}

		if err := s.collector.StopCheck(config.ShadowCheckID); err != nil && firstErr == nil {
			firstErr = err
		}
		handle.cleanup()
	}
	return firstErr
}

// Stop stops all scheduled shadow checks. Waiting for each check is bounded by
// the scheduler stop timeout.
func (s *ShadowScheduler) Stop() error {
	s.mu.Lock()
	s.stopped = true
	handles := make([]*shadowCheckHandle, 0, len(s.checks))
	for key, handle := range s.checks {
		handles = append(handles, handle)
		delete(s.checks, key)
		delete(s.checksByID, handle.check.ID())
	}
	s.stopped = true
	s.mu.Unlock()

	var firstErr error
	for _, handle := range handles {
		if err := s.collector.StopCheck(handle.check.ID()); err != nil && firstErr == nil {
			firstErr = err
		}
		handle.cleanup()
	}
	return firstErr
}

// IsCheckScheduled returns whether a shadow check is still scheduled.
func (s *ShadowScheduler) IsCheckScheduled(id checkid.ID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, found := s.checksByID[id]
	return found
}

type shadowKey struct {
	sourceConfigDigest string
	instanceIndex      int
}

func newShadowKey(config ShadowConfig) shadowKey {
	return shadowKey{
		sourceConfigDigest: config.SourceConfigDigest,
		instanceIndex:      config.InstanceIndex,
	}
}

type shadowCheckHandle struct {
	check         check.Check
	cancel        context.CancelFunc
	senderManager sender.SenderManager
	stopOnce      sync.Once
}

func (h *shadowCheckHandle) cleanup() {
	h.stopOnce.Do(func() {
		h.cancel()
		h.senderManager.DestroySender(h.check.ID())
		// Shadow runs reuse the runner expvar stats source under the :shadow ID.
		// Remove those stats on unschedule/stop so status does not show stale
		// shadow checks after their source config is gone.
		if _, found := expvars.CheckStats(h.check.ID()); found {
			expvars.RemoveCheckStats(h.check.ID())
		}
	})
}

type shadowCheck struct {
	check.Check
	id       checkid.ID
	interval time.Duration
}

func (c *shadowCheck) ID() checkid.ID {
	return c.id
}

func (c *shadowCheck) Interval() time.Duration {
	return c.interval
}

func (c *shadowCheck) IsShadow() bool {
	return true
}

type shadowSenderManager struct {
	delegate sender.SenderManager
	sourceID checkid.ID
	shadowID checkid.ID
}

func (m *shadowSenderManager) GetSender(id checkid.ID) (sender.Sender, error) {
	return m.delegate.GetSender(m.mapID(id))
}

func (m *shadowSenderManager) SetSender(s sender.Sender, id checkid.ID) error {
	return m.delegate.SetSender(s, m.mapID(id))
}

func (m *shadowSenderManager) DestroySender(id checkid.ID) {
	m.delegate.DestroySender(m.mapID(id))
}

func (m *shadowSenderManager) GetDefaultSender() (sender.Sender, error) {
	return m.delegate.GetDefaultSender()
}

func (m *shadowSenderManager) mapID(id checkid.ID) checkid.ID {
	if id == m.sourceID {
		return m.shadowID
	}
	return id
}
