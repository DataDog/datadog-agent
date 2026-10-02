// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package engine

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/report"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/telemetry"
)

func progressRequest(t *testing.T) Request {
	t.Helper()
	r := sharedBaselineRequest(t)
	r.Scenario.Fleet = r.Scenario.Fleet[:1]
	r.Scenario.Fleet[0].Count = 1
	r.Scenario.Phases[0].Duration.Duration = 20 * time.Second
	return requestFor(t, r.Scenario, schema.Digest([]byte("progress-fixture")), r.Bundle)
}

type progressDelivery struct {
	*recordingDelivery
	sendGate      <-chan struct{}
	sendEntered   chan schema.Stream
	drainGate     <-chan struct{}
	drainEntered  chan struct{}
	drainReturned chan struct{}
}

func (d *progressDelivery) Send(ctx context.Context, at time.Time, stream schema.Stream, samples []*telemetry.Sample) error {
	if d.sendEntered != nil {
		select {
		case d.sendEntered <- stream:
		default:
		}
	}
	if d.sendGate != nil {
		select {
		case <-d.sendGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return d.recordingDelivery.Send(ctx, at, stream, samples)
}

func (d *progressDelivery) Wait(ctx context.Context) error {
	if d.drainEntered != nil {
		close(d.drainEntered)
	}
	if d.drainReturned != nil {
		defer close(d.drainReturned)
	}
	if d.drainGate != nil {
		select {
		case <-d.drainGate:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return d.recordingDelivery.Wait(ctx)
}

type progressResult struct {
	report *report.Report
	err    error
}

func receiveProgressValue[T any](ctx context.Context, t *testing.T, values <-chan T) T {
	t.Helper()
	select {
	case value := <-values:
		return value
	case <-ctx.Done():
		t.Fatal("progress synchronization timed out:", ctx.Err())
		var zero T
		return zero
	}
}

func sendProgressTick(ctx context.Context, t *testing.T, ticks chan<- time.Time, clock Clock) {
	t.Helper()
	select {
	case ticks <- clock.Now():
	case <-ctx.Done():
		t.Fatal("progress observer stopped accepting ticks:", ctx.Err())
	}
}

func deliveredProgress(r *report.Report) uint64 {
	var delivered uint64
	for _, device := range r.Ledger {
		for _, counts := range device.Streams {
			delivered += counts.Delivered
		}
	}
	return delivered
}

func TestProgressContinuesUnderBackpressureAndOwnsSnapshots(t *testing.T) {
	r := progressRequest(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	clock := &advancingClock{now: r.Plan.Start}
	ticks := make(chan time.Time)
	snapshots := make(chan *report.Report, 4)
	sendGate := make(chan struct{})
	callbackGate := make(chan struct{})
	var blockCallback atomic.Bool
	delivery := &progressDelivery{recordingDelivery: &recordingDelivery{start: r.Plan.Start}, sendGate: sendGate, sendEntered: make(chan schema.Stream, 16)}
	done := make(chan progressResult, 1)
	go func() {
		result, err := Run(ctx, r, Options{Workers: 1, QueueCapacity: 1, Clock: clock, Delivery: delivery, progressTicks: ticks, Progress: func(snapshot *report.Report) error {
			snapshots <- snapshot
			if blockCallback.Swap(false) {
				select {
				case <-callbackGate:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		}})
		done <- progressResult{result, err}
	}()
	initial := receiveProgressValue(ctx, t, snapshots)
	if initial.Status != "running" || initial.Progress.Activity != "replaying" || deliveredProgress(initial) != 0 {
		t.Fatal("initial progress did not describe an undelivered running fleet")
	}
	firstStream := receiveProgressValue(ctx, t, delivery.sendEntered)
	// Delayed delivery must not prevent the independent clock from reporting
	// that the scenario window has elapsed.
	clock.mu.Lock()
	clock.now = r.Plan.Start.Add(25 * time.Second)
	clock.mu.Unlock()
	blockCallback.Store(true)
	sendProgressTick(ctx, t, ticks, clock)
	blocked := receiveProgressValue(ctx, t, snapshots)
	if deliveredProgress(blocked) != 0 || blocked.Progress.Elapsed < blocked.Progress.Duration || blocked.Progress.Activity != "waiting_for_delivery" || blocked.Progress.Phase != "healthy" {
		t.Fatal("blocked delivery was counted or suppressed the waiting heartbeat")
	}
	select {
	case sendGate <- struct{}{}:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// The first worker must account for its success and enter the next Send
	// while the progress callback is still blocked: the callback owns no lock.
	receiveProgressValue(ctx, t, delivery.sendEntered)
	close(callbackGate)
	sendProgressTick(ctx, t, ticks, clock)
	one := receiveProgressValue(ctx, t, snapshots)
	if deliveredProgress(one) != 1 || one.Ledger[0].Streams[firstStream].Delivered != 1 {
		t.Fatal("progress counted queued work instead of the single accepted cycle")
	}
	if deliveredProgress(initial) != 0 || deliveredProgress(blocked) != 0 {
		t.Fatal("retained snapshots changed when the worker accepted a later cycle")
	}
	close(sendGate)
	result := receiveProgressValue(ctx, t, done)
	if result.err != nil || !result.report.Complete() || result.report.Progress.Activity != "succeeded" {
		t.Fatal("progress observation prevented complete replay:", result.err)
	}
	if deliveredProgress(initial) != 0 || deliveredProgress(one) != 1 {
		t.Fatal("final accounting mutated a previously retained snapshot")
	}
	select {
	case ticks <- clock.Now():
		t.Fatal("progress observer is still receiving ticks after replay completed")
	default:
	}
}

func TestProgressCallbackFailureCancelsReplay(t *testing.T) {
	for _, initialFailure := range []bool{true, false} {
		name := "periodic"
		if initialFailure {
			name = "initial"
		}
		t.Run(name, func(t *testing.T) {
			r := progressRequest(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			clock := &advancingClock{now: r.Plan.Start}
			ticks := make(chan time.Time)
			snapshots := make(chan *report.Report, 2)
			delivery := &progressDelivery{recordingDelivery: &recordingDelivery{start: r.Plan.Start}, sendGate: make(chan struct{}), sendEntered: make(chan schema.Stream, 2)}
			failure := errors.New("injected progress writer failure")
			var calls int
			done := make(chan progressResult, 1)
			go func() {
				result, err := Run(ctx, r, Options{Workers: 1, QueueCapacity: 1, Clock: clock, Delivery: delivery, progressTicks: ticks, Progress: func(snapshot *report.Report) error {
					calls++
					snapshots <- snapshot
					if initialFailure || calls > 1 {
						return failure
					}
					return nil
				}})
				done <- progressResult{result, err}
			}()
			initial := receiveProgressValue(ctx, t, snapshots)
			if !initialFailure {
				receiveProgressValue(ctx, t, delivery.sendEntered)
				sendProgressTick(ctx, t, ticks, clock)
			}
			result := receiveProgressValue(ctx, t, done)
			if !errors.Is(result.err, failure) || result.report == nil || result.report.Status != "failed" || result.report.Progress.Activity != "failed" || len(result.report.Errors) == 0 {
				t.Fatal("progress failure was not retained in the final failed report:", result.err)
			}
			if deliveredProgress(result.report) != 0 || deliveredProgress(initial) != 0 || result.report.Complete() {
				t.Fatal("cancellation counted the blocked Send as delivered or mutated the initial snapshot")
			}
			if initialFailure {
				select {
				case <-delivery.sendEntered:
					t.Fatal("delivery began after initial progress publication failed")
				default:
				}
			}
		})
	}
}

func TestProgressContinuesThroughFinalDrainAndJoinsObserver(t *testing.T) {
	for _, observerFails := range []bool{false, true} {
		name := "success"
		if observerFails {
			name = "observer_failure"
		}
		t.Run(name, func(t *testing.T) {
			r := progressRequest(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			clock := &advancingClock{now: r.Plan.Start}
			ticks := make(chan time.Time)
			snapshots := make(chan *report.Report, 2)
			drainGate := make(chan struct{})
			callbackGate := make(chan struct{})
			callbackExited := make(chan struct{})
			delivery := &progressDelivery{recordingDelivery: &recordingDelivery{start: r.Plan.Start}, drainGate: drainGate, drainEntered: make(chan struct{}), drainReturned: make(chan struct{})}
			failure := errors.New("injected progress failure during final drain")
			var calls int
			done := make(chan progressResult, 1)
			go func() {
				result, err := Run(ctx, r, Options{Workers: 1, QueueCapacity: 1, Clock: clock, Delivery: delivery, progressTicks: ticks, Progress: func(snapshot *report.Report) error {
					calls++
					snapshots <- snapshot
					if calls == 1 {
						return nil
					}
					defer close(callbackExited)
					select {
					case <-callbackGate:
					case <-ctx.Done():
						return ctx.Err()
					}
					if observerFails {
						return failure
					}
					return nil
				}})
				done <- progressResult{result, err}
			}()
			initial := receiveProgressValue(ctx, t, snapshots)
			receiveProgressValue(ctx, t, delivery.drainEntered)
			sendProgressTick(ctx, t, ticks, clock)
			draining := receiveProgressValue(ctx, t, snapshots)
			if draining.Status != "running" || draining.Progress.Elapsed < draining.Progress.Duration || draining.Progress.Activity != "waiting_for_delivery" || draining.Progress.Phase != "healthy" {
				t.Fatal("final delivery drain stopped the progress heartbeat")
			}
			for _, device := range draining.Ledger {
				for _, counts := range device.Streams {
					if counts.Delivered != counts.Expected || counts.Failed != 0 {
						t.Fatal("drain heartbeat lost already accepted cycles")
					}
				}
			}
			close(drainGate)
			receiveProgressValue(ctx, t, delivery.drainReturned)
			select {
			case <-done:
				t.Fatal("replay returned while its progress callback was still running")
			default:
			}
			close(callbackGate)
			result := receiveProgressValue(ctx, t, done)
			select {
			case <-callbackExited:
			default:
				t.Fatal("replay did not join the progress callback")
			}
			if observerFails {
				if !errors.Is(result.err, failure) || result.report.Status != "failed" || result.report.Progress.Activity != "failed" {
					t.Fatal("final progress failure was lost after delivery drained:", result.err)
				}
			} else if result.err != nil || !result.report.Complete() || result.report.Progress.Activity != "succeeded" {
				t.Fatal("successful drain did not produce final success:", result.err)
			}
			if deliveredProgress(initial) != 0 || deliveredProgress(result.report) != deliveredProgress(draining) {
				t.Fatal("observer shutdown corrupted snapshot or delivery accounting")
			}
			select {
			case ticks <- clock.Now():
				t.Fatal("observer accepted a heartbeat after final accounting")
			default:
			}
		})
	}
}
