// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startupsequencerimpl implements the startupsequencer component.
package startupsequencerimpl

import (
	"context"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	startupsequencer "github.com/DataDog/datadog-agent/comp/core/startupsequencer/def"
)

// Requires defines the dependencies of the startupsequencer component.
type Requires struct {
	Config config.Component
	Log    log.Component
}

// Provides defines the output of the startupsequencer component.
type Provides struct {
	Comp startupsequencer.Component
}

type deferredStart struct {
	name string
	fn   func(context.Context) error
}

type sequencer struct {
	log          log.Component
	enabled      bool
	interval     time.Duration
	freeOSMemory bool

	mu       sync.Mutex
	begun    bool
	deferred [startupsequencer.NumStages][]deferredStart
}

// NewComponent returns the staged startup sequencer.
func NewComponent(reqs Requires) Provides {
	return Provides{
		Comp: &sequencer{
			log:          reqs.Log,
			enabled:      reqs.Config.GetBool("staged_start.enabled"),
			interval:     reqs.Config.GetDuration("staged_start.stage_interval"),
			freeOSMemory: reqs.Config.GetBool("staged_start.free_os_memory"),
		},
	}
}

func (s *sequencer) Defer(stage startupsequencer.Stage, name string, fn func(context.Context) error) error {
	if !s.enabled {
		// Run inline so behavior is identical to performing the work directly
		// in the caller's OnStart hook.
		return fn(context.Background())
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.begun {
		// Registered after the sequence already started: run inline rather than
		// silently dropping the work.
		s.log.Warnf("staged startup: %q registered after sequence began; running inline", name)
		return fn(context.Background())
	}
	s.deferred[stage] = append(s.deferred[stage], deferredStart{name: name, fn: fn})
	return nil
}

func (s *sequencer) Begin(ctx context.Context) {
	if !s.enabled {
		return
	}

	s.mu.Lock()
	if s.begun {
		s.mu.Unlock()
		return
	}
	s.begun = true
	deferred := s.deferred
	s.mu.Unlock()

	go s.run(ctx, deferred)
}

func (s *sequencer) run(ctx context.Context, deferred [startupsequencer.NumStages][]deferredStart) {
	s.log.Infof("staged startup: beginning (stage interval %s)", s.interval)
	for stage := startupsequencer.Stage(0); stage < startupsequencer.NumStages; stage++ {
		for _, d := range deferred[stage] {
			if ctx.Err() != nil {
				s.log.Infof("staged startup: aborted before %q (context cancelled)", d.name)
				return
			}
			start := time.Now()
			if err := d.fn(ctx); err != nil {
				s.log.Errorf("staged startup: %q (stage %d) failed: %v", d.name, stage, err)
			} else {
				s.log.Debugf("staged startup: started %q (stage %d) in %s", d.name, stage, time.Since(start))
			}
		}

		// Return transient memory allocated during this stage to the OS before
		// the next stage allocates, keeping the peak RSS close to steady state.
		if s.freeOSMemory {
			runtime.GC()
			debug.FreeOSMemory()
		}

		if stage < startupsequencer.NumStages-1 {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.interval):
			}
		}
	}
	s.log.Info("staged startup: complete")
}
