// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package hook provides a lightweight, generic publish/subscribe mechanism that
// allows any internal or external part of the Agent to observe data flowing
// through existing pipelines without modifying the pipeline code.
//
// # Motivation
//
// Core Agent pipelines (metric aggregation, DogStatsD, trace processing, …) are
// performance-critical and must not be slowed down by optional consumers.  The
// hook package satisfies three hard constraints:
//
//  1. The producer (pipeline goroutine) is never blocked by a slow consumer.
//  2. Adding or removing a hook adds zero allocation on the hot path when no
//     subscriber is registered, and zero allocation per-subscriber when the
//     subscriber's buffer has space.
//  3. A slow or stalled consumer only loses its own payloads; other consumers
//     and the producer are unaffected.
//
// # Typical usage
//
// A hook is created once (typically in an fx constructor) and published to an
// fx group so that any component can subscribe to it:
//
//	// producer side — e.g. inside the demultiplexer
//	metricHook := hook.NewHook[observer.MetricView]("metrics-pipeline")
//	// metricHook is injected into the fx graph under group:"hook"
//
//	// in TimeSampler, CheckSampler, AggregatingSender …
//	metricHook.Publish("time-sampler", sample)
//
//	// consumer side — e.g. the shared-memory ring-buffer writer
//	unsubscribe := metricHook.Subscribe("shm-writer", func(v observer.MetricView) {
//	    ringbuf.Write(v.Name(), v.Value(), v.Tags())  // must return quickly
//	})
//	defer unsubscribe()
//
// # Delivery guarantees
//
// Each subscriber owns a buffered channel (capacity 100).  Publish enqueues
// the payload into every subscriber's channel under an RLock, returning
// immediately.  If a subscriber's channel is full the payload is silently
// dropped for that subscriber and a telemetry counter is incremented; all
// other subscribers still receive the payload.
//
// The subscriber callback runs in a dedicated goroutine.  It must return
// promptly to avoid filling the buffer.
//
// # Copy safety
//
// Many pipeline objects (e.g. [pkg/metrics.MetricSample]) are pooled and may
// be recycled by the producer after Publish returns.  The subscriber callback
// must copy any data it needs to retain before returning, as the underlying
// object may be reused by the time the callback is called again.
//
// # Telemetry
//
// The package exposes three Prometheus metrics under the "hooks" subsystem:
//
//   - hooks_gauge{hook_name}                    — number of live hooks
//   - subscribed_callbacks_gauge{hook_name}     — number of active subscribers
//   - drops_counter{hook_name, consumer_name}   — payloads dropped per consumer
package hook
