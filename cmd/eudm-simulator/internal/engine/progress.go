// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
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

func writeProgress(w io.Writer, r *report.Report) error {
	if w == nil {
		return nil
	}
	counts := make(map[schema.Stream]report.Counts)
	add := func(stream schema.Stream, value *report.Counts) {
		total := counts[stream]
		total.Expected += value.Expected
		total.Delivered += value.Delivered
		total.Failed += value.Failed
		counts[stream] = total
	}
	for _, device := range r.Ledger {
		for stream, value := range device.Streams {
			add(stream, value)
		}
	}
	for stream, value := range r.NetworkStreams {
		add(stream, value)
	}
	streams := make([]schema.Stream, 0, len(counts))
	for stream := range counts {
		streams = append(streams, stream)
	}
	slices.Sort(streams)
	var line strings.Builder
	p := r.Progress
	activity := p.Activity
	if activity == "waiting_for_delivery" {
		activity = "waiting for delivery/retries"
	}
	fmt.Fprintf(&line, "Replay %s/%s | phase=%s | %s | delivered cycles:", p.Elapsed.Truncate(time.Second), p.Duration, p.Phase, activity)
	var failed uint64
	for _, stream := range streams {
		value := counts[stream]
		fmt.Fprintf(&line, " %s=%d/%d", stream, value.Delivered, value.Expected)
		failed += value.Failed
	}
	fmt.Fprintf(&line, " | failed=%d\n", failed)
	_, err := io.WriteString(w, line.String())
	return err
}
