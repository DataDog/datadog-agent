// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux && nvml

package safenvml

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	migPollInterval     = 5 * time.Second
	migFallbackUnpause  = 30 * time.Second
	migPendingLabelName = "nvidia.com/mig.config.state"
	migPendingLabelVal  = "pending"
)

var (
	// ErrNvmlPaused is returned when NVML calls are blocked due to a MIG reconfiguration.
	ErrNvmlPaused = errors.New("nvml paused due to mig reconfiguration")
	procfsRoot    = "/proc"
)

type migConfigStateProvider func(context.Context) (string, bool, error)

var migConfigStateSource struct {
	mu       sync.RWMutex
	provider migConfigStateProvider
}

// SetMigConfigStateProvider configures a provider for the MIG config state label.
func SetMigConfigStateProvider(provider func(context.Context) (string, bool, error)) {
	migConfigStateSource.mu.Lock()
	defer migConfigStateSource.mu.Unlock()
	migConfigStateSource.provider = migConfigStateProvider(provider)
}

func getMigConfigState(ctx context.Context) (string, bool, error) {
	migConfigStateSource.mu.RLock()
	provider := migConfigStateSource.provider
	migConfigStateSource.mu.RUnlock()
	if provider == nil {
		return "", false, nil
	}

	return provider(ctx)
}

var nvmlStateTelemetryStore struct {
	mu        sync.RWMutex
	telemetry *NvmlStateTelemetry
}

var telemetrySubscriberOnce sync.Once

type pauseEvent struct {
	Action     string
	Reason     string
	Paused     bool
	MigPending bool
	PauseSince time.Time
}

type pauseController struct {
	mu          sync.Mutex
	paused      bool
	migPending  bool
	pauseSince  time.Time
	subscribers []chan pauseEvent
}

func newPauseController() *pauseController {
	return &pauseController{}
}

func (c *pauseController) Subscribe() <-chan pauseEvent {
	ch := make(chan pauseEvent, 1)
	c.mu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.mu.Unlock()
	return ch
}

func (c *pauseController) IsPaused() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.paused
}

func (c *pauseController) IsMigPending() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.migPending
}

func (c *pauseController) PauseAge() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pauseSince.IsZero() {
		return 0
	}
	return time.Since(c.pauseSince)
}

func (c *pauseController) Pause(reason string) {
	c.mu.Lock()
	if c.paused {
		c.mu.Unlock()
		return
	}
	c.paused = true
	c.migPending = true
	c.pauseSince = time.Now()
	event := pauseEvent{
		Action:     "pause",
		Reason:     reason,
		Paused:     true,
		MigPending: true,
		PauseSince: c.pauseSince,
	}
	subscribers := append([]chan pauseEvent(nil), c.subscribers...)
	c.mu.Unlock()

	c.publish(subscribers, event)
}

func (c *pauseController) TryUnpause(reason string) {
	c.mu.Lock()
	if !c.paused {
		c.mu.Unlock()
		return
	}
	c.paused = false
	c.migPending = false
	c.pauseSince = time.Time{}
	event := pauseEvent{
		Action:     "unpause",
		Reason:     reason,
		Paused:     false,
		MigPending: false,
	}
	subscribers := append([]chan pauseEvent(nil), c.subscribers...)
	c.mu.Unlock()

	c.publish(subscribers, event)
}

func (c *pauseController) publish(subscribers []chan pauseEvent, event pauseEvent) {
	for _, ch := range subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// SetNvmlStateTelemetry registers a telemetry sink for NVML state changes.
func SetNvmlStateTelemetry(state *NvmlStateTelemetry) {
	nvmlStateTelemetryStore.mu.Lock()
	defer nvmlStateTelemetryStore.mu.Unlock()
	nvmlStateTelemetryStore.telemetry = state
	startTelemetrySubscriber()
	if state != nil {
		controller := getPauseController()
		state.SetPaused(controller.IsPaused())
		state.SetMigPending(controller.IsMigPending())
	}
}

func getNvmlStateTelemetry() *NvmlStateTelemetry {
	nvmlStateTelemetryStore.mu.RLock()
	defer nvmlStateTelemetryStore.mu.RUnlock()
	return nvmlStateTelemetryStore.telemetry
}

func pausedError(op string) error {
	return fmt.Errorf("%s: %w", op, ErrNvmlPaused)
}

func (s *safeNvml) shutdownLib() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lib == nil {
		return nil
	}

	if _, ok := s.capabilities["nvmlShutdown"]; ok {
		if retErr := NewNvmlAPIErrorOrNil("Shutdown", s.lib.Shutdown()); retErr != nil {
			s.lib = nil
			s.capabilities = nil
			return retErr
		}
	}

	s.lib = nil
	s.capabilities = nil
	return nil
}

func (s *safeNvml) getPauseController() *pauseController {
	s.pauseOnce.Do(func() {
		s.controller = newPauseController()
	})
	return s.controller
}

func getPauseController() *pauseController {
	return singleton.getPauseController()
}

func startTelemetrySubscriber() {
	telemetrySubscriberOnce.Do(func() {
		events := getPauseController().Subscribe()
		go func() {
			for event := range events {
				telemetry := getNvmlStateTelemetry()
				if telemetry == nil {
					continue
				}
				telemetry.SetPaused(event.Paused)
				telemetry.SetMigPending(event.MigPending)
				telemetry.AddPauseTransition(event.Action, event.Reason)
			}
		}()
	})
}

func (s *safeNvml) startPauseSubscriber() {
	s.pauseEvents.Do(func() {
		events := s.getPauseController().Subscribe()
		go func() {
			for event := range events {
				if event.Paused {
					if err := s.shutdownLib(); err != nil {
						log.Warnf("error shutting down NVML lib during pause (%s): %v", event.Reason, err)
					}
					log.Debugf("NVML pause requested: %s", event.Reason)
					continue
				}
				log.Debugf("NVML pause cleared: %s", event.Reason)
			}
		}()
	})
}

func (s *safeNvml) startMigPoller() {
	s.getPauseController()
	s.pollerOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(migPollInterval)
			defer ticker.Stop()
			for {
				s.pollMigState()
				<-ticker.C
			}
		}()
	})
}

func (s *safeNvml) pollMigState() {
	controller := s.getPauseController()
	ctx := context.Background()
	labelState, labelPresent, labelErr := getMigConfigState(ctx)
	if labelErr != nil {
		log.Debugf("error reading MIG config state label: %v", labelErr)
	}

	if labelPresent {
		if strings.EqualFold(labelState, migPendingLabelVal) {
			controller.Pause("label_pending")
			return
		}
		controller.TryUnpause("label_ready")
		return
	}

	if controller.IsPaused() {
		migInstances := hasMigInstances(procfsRoot)
		if telemetry := getNvmlStateTelemetry(); telemetry != nil {
			telemetry.SetMigInstancesPresent(migInstances)
		}
		if migInstances {
			controller.TryUnpause("procfs_mig_instances")
			return
		}
		if controller.PauseAge() >= migFallbackUnpause {
			controller.TryUnpause("fallback_timer")
		}
		return
	}

	s.pollNvmlMigMode()
}

func (s *safeNvml) pollNvmlMigMode() {
	controller := s.getPauseController()
	count, err := s.DeviceGetCount()
	if err != nil {
		return
	}

	migPending := false
	for i := 0; i < count; i++ {
		dev, err := s.DeviceGetHandleByIndex(i)
		if err != nil {
			continue
		}
		current, pending, err := dev.GetMigMode()
		if err != nil {
			continue
		}
		if current != pending {
			migPending = true
			break
		}
	}

	if telemetry := getNvmlStateTelemetry(); telemetry != nil {
		telemetry.SetMigPending(migPending)
	}

	if migPending {
		controller.Pause("nvml_pending")
	}
}

func hasMigInstances(root string) bool {
	pattern := filepath.Join(root, "driver", "nvidia", "capabilities", "gpu*", "mig", "gi*", "ci*", "access")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return false
	}
	return len(matches) > 0
}
