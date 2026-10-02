// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
)

const progressInterval = 30 * time.Second

func progressAt(r *report.Report, at time.Time) *report.Progress {
	p := &report.Progress{UpdatedAt: at, Elapsed: max(0, at.Sub(r.Start)), Activity: "replaying"}
	for _, phase := range r.Phases {
		p.Duration += phase.Duration
		if p.Elapsed >= phase.StartOffset {
			p.Phase = phase.Name
		}
	}
	if r.Status != "running" {
		p.Activity = r.Status
	} else if p.Elapsed >= p.Duration {
		p.Activity = "waiting_for_delivery"
	}
	return p
}

// Observe independently of scheduling and delivery, including queue backpressure
// and the final drain. Publish owned snapshots without holding the workers' lock.
// The returned function joins the observer before final accounting is changed.
func observeProgress(ctx context.Context, r *report.Report, mu *sync.Mutex, options Options, fail func(error)) func() {
	if options.Progress == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	publish := func() bool {
		mu.Lock()
		snapshot := r.Snapshot()
		mu.Unlock()
		snapshot.Progress = progressAt(snapshot, options.Clock.Now())
		if err := options.Progress(snapshot); err != nil {
			fail(fmt.Errorf("publish replay progress: %w", err))
			return false
		}
		return true
	}
	if !publish() {
		cancel()
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticks := options.progressTicks
		if ticks == nil {
			ticker := time.NewTicker(progressInterval)
			defer ticker.Stop()
			ticks = ticker.C
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				if ctx.Err() != nil || !publish() {
					return
				}
			}
		}
	}()
	return func() { cancel(); <-done }
}
