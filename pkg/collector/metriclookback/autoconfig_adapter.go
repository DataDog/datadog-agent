// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metriclookback

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// ShadowController schedules derived lookback shadow configs. Stop may
// race with an in-flight Schedule call; implementations must prevent late-start
// after Stop returns.
type ShadowController interface {
	Schedule([]ShadowConfig) error
	Unschedule([]ShadowConfig) error
	Stop() error
}

// AutoConfigShadowAdapter adapts AutoConfig scheduler callbacks to the shadow
// scheduler. AutoConfig invokes scheduler callbacks while holding its controller
// lock, so this adapter only derives shadow configs and enqueues lifecycle work.
type AutoConfigShadowAdapter struct {
	opts       Options
	controller ShadowController

	mu      sync.Mutex
	queue   []shadowLifecycleWork
	stopped bool
	notify  chan struct{}
}

// NewAutoConfigShadowAdapter creates an AutoConfig-facing scheduler for lookback
// shadow checks.
func NewAutoConfigShadowAdapter(opts Options, controller ShadowController) *AutoConfigShadowAdapter {
	s := &AutoConfigShadowAdapter{
		opts:       opts,
		controller: controller,
		notify:     make(chan struct{}, 1),
	}
	go s.run()
	return s
}

// Schedule derives shadow configs and enqueues them for asynchronous scheduling.
func (s *AutoConfigShadowAdapter) Schedule(configs []integration.Config) {
	s.enqueue(shadowLifecycleSchedule, DeriveShadowConfigs(configs, s.opts))
}

// Unschedule derives shadow configs and enqueues them for asynchronous removal.
func (s *AutoConfigShadowAdapter) Unschedule(configs []integration.Config) {
	s.enqueue(shadowLifecycleUnschedule, DeriveShadowConfigs(configs, s.opts))
}

// Stop stops accepting lifecycle work, drops queued work, and stops the
// underlying shadow controller. It does not wait for a worker already inside the
// shadow scheduler; ShadowController is responsible for making that
// in-flight work self-clean instead of starting after Stop.
func (s *AutoConfigShadowAdapter) Stop() {
	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.stopped = true
	s.queue = nil
	s.mu.Unlock()

	s.signal()
	if err := s.controller.Stop(); err != nil {
		log.Warnf("failed to stop metric lookback shadow scheduler: %v", err)
	}
}

func (s *AutoConfigShadowAdapter) enqueue(action shadowLifecycleAction, configs []ShadowConfig) {
	if len(configs) == 0 {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	s.queue = append(s.queue, shadowLifecycleWork{action: action, configs: configs})
	s.mu.Unlock()

	s.signal()
}

func (s *AutoConfigShadowAdapter) signal() {
	select {
	case s.notify <- struct{}{}:
	default:
	}
}

func (s *AutoConfigShadowAdapter) run() {
	for {
		work, ok := s.next()
		if !ok {
			return
		}

		var err error
		switch work.action {
		case shadowLifecycleSchedule:
			err = s.controller.Schedule(work.configs)
		case shadowLifecycleUnschedule:
			err = s.controller.Unschedule(work.configs)
		}
		if err != nil {
			log.Warnf("failed to apply metric lookback shadow lifecycle update: %v", err)
		}
	}
}

func (s *AutoConfigShadowAdapter) next() (shadowLifecycleWork, bool) {
	for {
		s.mu.Lock()
		if len(s.queue) > 0 {
			work := s.queue[0]
			s.queue[0] = shadowLifecycleWork{}
			s.queue = s.queue[1:]
			s.mu.Unlock()
			return work, true
		}
		if s.stopped {
			s.mu.Unlock()
			return shadowLifecycleWork{}, false
		}
		s.mu.Unlock()

		<-s.notify
	}
}

type shadowLifecycleAction int

const (
	shadowLifecycleSchedule shadowLifecycleAction = iota
	shadowLifecycleUnschedule
)

type shadowLifecycleWork struct {
	action  shadowLifecycleAction
	configs []ShadowConfig
}
