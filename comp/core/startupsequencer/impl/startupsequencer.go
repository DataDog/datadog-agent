// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package startupsequencerimpl implements the startupsequencer component.
package startupsequencerimpl

import (
	"context"
	"sync"
	"time"

	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	startupsequencer "github.com/DataDog/datadog-agent/comp/core/startupsequencer/def"
	"github.com/DataDog/datadog-agent/pkg/util/stagedstart"
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
	log     log.Component
	enabled bool
	pacer   *stagedstart.Pacer

	mu       sync.Mutex
	begun    bool
	deferred [startupsequencer.NumStages][]deferredStart
}

// NewComponent returns the staged startup sequencer.
func NewComponent(reqs Requires) Provides {
	cfg := stagedstart.ConfigFromReader(reqs.Config)
	return Provides{
		Comp: &sequencer{
			log:     reqs.Log,
			enabled: cfg.Enabled,
			pacer: stagedstart.NewPacer(cfg,
				func(f string, a ...interface{}) { reqs.Log.Infof(f, a...) },
				func(f string, a ...interface{}) { reqs.Log.Warnf(f, a...) },
			),
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
	// Flatten stages into a single ordered list. Stages are only a coarse
	// priority (critical-path subsystems first); the pacer keeps consecutive
	// items from allocating on top of one another, so the exact order within
	// that priority does not affect the memory peak.
	var items []deferredStart
	for stage := startupsequencer.Stage(0); stage < startupsequencer.NumStages; stage++ {
		items = append(items, deferred[stage]...)
	}

	s.log.Info("staged startup: beginning")
	for i, d := range items {
		if ctx.Err() != nil {
			s.log.Infof("staged startup: aborted before %q (context cancelled)", d.name)
			return
		}
		start := time.Now()
		if err := d.fn(ctx); err != nil {
			s.log.Errorf("staged startup: %q failed: %v", d.name, err)
		} else {
			s.log.Debugf("staged startup: started %q in %s", d.name, time.Since(start))
		}

		// Pace before releasing the next item (reclaims transient memory and,
		// in adaptive mode, waits until this item's allocation settles).
		if i < len(items)-1 {
			s.pacer.Pace(ctx, d.name)
		}
	}
	s.log.Info("staged startup: complete")
}
