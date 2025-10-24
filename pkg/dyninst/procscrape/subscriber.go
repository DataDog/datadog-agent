// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

// Package procscrape implements a subscriber that bridges procmon and rcscrape.
// It is responsible for coalescing process lifecycle updates and remote-config
// updates into a single stream of Updates.
package procscrape

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	processesUpdateInterval = 200 * time.Millisecond
	processSyncInterval     = 30 * time.Second
)

// EventSource describes the ability to subscribe to exec and exit events for
// processes. Implementations are expected to invoke the provided callbacks
// synchronously when the corresponding events occur, and return a function that
// removes the subscription when invoked.
type EventSource interface {
	SubscribeExec(func(pid uint32)) (cleanup func())
	SubscribeExit(func(pid uint32)) (cleanup func())
}

// Update captures both process removals and refreshed configurations.
type Update = process.ProcessesUpdate

// Subscriber bridges procmon and rcscrape, coalescing lifecycle notifications
// and remote-config updates into a single stream of Update values.
//
// The expectation is that Subscribe must be called before Start.
type Subscriber struct {
	started        bool
	processEvents  EventSource
	procMon        *procmon.ProcessMonitor
	scraper        *rcscrape.Scraper
	scraperHandler procmon.Handler
	syncDisabled   bool

	callback func(Update)

	stop      sync.Once
	start     sync.Once
	cancel    context.CancelFunc
	unsubExec context.CancelFunc
	unsubExit context.CancelFunc

	wg sync.WaitGroup
}

var _ procmon.Handler = (*Subscriber)(nil)

// NewSubscriber creates a new Subscriber tied to the provided scraper and
// process watcher.
func NewSubscriber(
	scraper *rcscrape.Scraper,
	watcher EventSource,
	syncDisabled bool,
) *Subscriber {
	return &Subscriber{
		processEvents:  watcher,
		scraper:        scraper,
		scraperHandler: scraper.AsProcMonHandler(),
		syncDisabled:   syncDisabled,
	}
}

// Subscribe registers a callback that will receive future updates.
//
// Note that if called after Start, this function is a no-op.
func (s *Subscriber) Subscribe(cb func(Update)) {
	if s.started {
		return
	}
	s.callback = cb
}

// Start begins delivering updates to the registered callback.
func (s *Subscriber) Start() {
	s.start.Do(func() {
		if s.callback == nil {
			s.callback = func(Update) {}
		}
		s.started = true
		var ctx context.Context
		ctx, s.cancel = context.WithCancel(context.Background())
		s.procMon = procmon.NewProcessMonitor(s)
		s.unsubExec = s.processEvents.SubscribeExec(s.procMon.NotifyExec)
		s.unsubExit = s.processEvents.SubscribeExit(s.procMon.NotifyExit)
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.runUpdateLoop(ctx)
		}()
		if !s.syncDisabled {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.runSyncLoop(ctx)
			}()
		}
	})
}

// Close stops delivering updates and releases associated resources.
func (s *Subscriber) Close() {
	var notStarted bool
	if s.start.Do(func() { notStarted = true }); notStarted {
		return
	}
	s.stop.Do(func() {
		s.unsubExec()
		s.unsubExit()
		s.cancel()
		s.procMon.Close()
		s.wg.Wait()
	})
}

// HandleUpdate implements procmon.Handler, relaying removals to the callback.
func (s *Subscriber) HandleUpdate(update procmon.ProcessesUpdate) {
	s.scraperHandler.HandleUpdate(update)
	if len(update.Removals) > 0 {
		s.callback(Update{Removals: update.Removals})
	}
}

func (s *Subscriber) runUpdateLoop(ctx context.Context) {
	duration := func() time.Duration { return jitter(processesUpdateInterval, 0.2) }
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if updates := s.scraper.GetUpdates(); len(updates) > 0 {
				s.callback(Update{Updates: updates})
			}
		}
		timer.Reset(duration())
	}
}

func (s *Subscriber) runSyncLoop(ctx context.Context) {
	timer := time.NewTimer(0)
	defer timer.Stop()

	duration := func() time.Duration { return jitter(processSyncInterval, 0.2) }

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := s.procMon.Sync(); err != nil {
				log.Errorf("error syncing process monitor: %v", err)
			}
			timer.Reset(duration())
		}
	}
}

func jitter(duration time.Duration, fraction float64) time.Duration {
	multiplier := 1 + ((rand.Float64()*2 - 1) * fraction)
	return time.Duration(float64(duration) * multiplier)
}
