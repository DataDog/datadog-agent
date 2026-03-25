// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"context"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/config"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	flightrecorder "github.com/DataDog/datadog-agent/comp/flightrecorder/def"
	"github.com/DataDog/datadog-agent/pkg/hook"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// tagPool recycles string slices used for metric/log tag copies.
// Pre-capped to 32 strings to cover the common case without re-allocation.
var tagPool = sync.Pool{New: func() any { s := make([]string, 0, 32); return &s }}

// contentPool recycles byte slices used for log content copies.
var contentPool = sync.Pool{New: func() any { s := make([]byte, 0, 256); return &s }}

// Requires defines the dependencies injected into the flight recorder component.
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component

	MetricsHooks    []hook.Hook[hook.MetricView]     `group:"hook"`
	LogsHooks       []hook.Hook[hook.LogView]        `group:"hook"`
	TraceStatsHooks []hook.Hook[hook.TraceStatsView] `group:"hook"`
}

// Provides defines what the component exposes to the fx graph.
type Provides struct {
	Comp flightrecorder.Component
}

// sinkImpl implements flightrecorder.Component.
type sinkImpl struct {
	counters *counters
}

func (s *sinkImpl) Stats() flightrecorder.Stats {
	return s.counters.stats()
}

// noopSink satisfies the Component interface when flightrecorder is disabled.
type noopSink struct{}

func (noopSink) Stats() flightrecorder.Stats { return flightrecorder.Stats{} }

// NewComponent creates the flight recorder fx component.
func NewComponent(req Requires) (Provides, error) {
	if !req.Config.GetBool("flightrecorder.enabled") {
		pkglog.Debug("flightrecorder: disabled (flightrecorder.enabled=false)")
		return Provides{Comp: noopSink{}}, nil
	}

	socketPath := req.Config.GetString("flightrecorder.socket_path")
	flushInterval := req.Config.GetDuration("flightrecorder.flush_interval")
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}
	ptCapacity := req.Config.GetInt("flightrecorder.point_buffer_capacity")
	if ptCapacity <= 0 {
		ptCapacity = 20000
	}
	defCapacity := req.Config.GetInt("flightrecorder.def_buffer_capacity")
	if defCapacity <= 0 {
		defCapacity = 2000
	}
	logCapacity := req.Config.GetInt("flightrecorder.log_buffer_capacity")
	if logCapacity <= 0 {
		logCapacity = 5000
	}
	traceStatsCapacity := req.Config.GetInt("flightrecorder.trace_stats_buffer_capacity")
	if traceStatsCapacity <= 0 {
		traceStatsCapacity = 1000
	}
	hookBufSize := req.Config.GetInt("flightrecorder.hook_buffer_size")
	if hookBufSize <= 0 {
		hookBufSize = 4096
	}
	contextCap := req.Config.GetInt("flightrecorder.context_set_capacity")
	if contextCap <= 0 {
		contextCap = 500000
	}

	c := &counters{}
	// Create transport and batcher. The reconnect callback is set after
	// batcher creation to avoid a circular dependency.
	transport := newUnixTransport(socketPath, func() { c.incReconnects() })
	bat := newBatcher(transport, flushInterval, ptCapacity, defCapacity, logCapacity, traceStatsCapacity, contextCap, c)
	// Replace the onReconnect callback to also reset context definitions.
	transport.mu.Lock()
	transport.onReconnect = func() {
		c.incReconnects()
		bat.ResetContexts()
	}
	transport.mu.Unlock()

	impl := &sinkImpl{counters: c}

	// Subscribe to all metric hooks:
	//   dogstatsd-pipeline — raw DogStatsD samples (pre-aggregation, per UDP packet)
	//   metrics-pipeline   — post-aggregated series: DogStatsD (time-sampler),
	//                        check metrics (check_sampler), no-agg pipeline
	for _, mh := range req.MetricsHooks {
		if mh == nil {
			continue
		}
		source := mh.Name()
		mh.SubscribeWithBuffer("flightrecorder-metrics", hookBufSize, func(payload hook.MetricView) {
			name := payload.GetName()
			raw := payload.GetRawTags()
			ckey := computeContextKey(name, raw)
			ts := int64(payload.GetTimestamp() * float64(time.Second/time.Nanosecond))

			if bat.IsContextKnown(ckey) {
				// Fast path: context already sent — compact point, no string copies.
				bat.AddPoint(metricPoint{
					ContextKey:  ckey,
					Value:       payload.GetValue(),
					TimestampNs: ts,
					SampleRate:  payload.GetSampleRate(),
					Source:      source,
				})
			} else {
				// Slow path: first occurrence — copy strings for context definition.
				sp := tagPool.Get().(*[]string)
				tags := append((*sp)[:0], raw...)
				bat.AddContextDef(contextDef{
					ContextKey:   ckey,
					Name:         name,
					Value:        payload.GetValue(),
					Tags:         tags,
					TagPoolSlice: sp,
					TimestampNs:  ts,
					SampleRate:   payload.GetSampleRate(),
					Source:       source,
				})
			}
		})
	}

	// Subscribe to all log hooks.
	for _, lh := range req.LogsHooks {
		if lh == nil {
			continue
		}
		lh.SubscribeWithBuffer("flightrecorder-logs", hookBufSize, func(payload hook.LogView) {
			raw := payload.GetContent()
			cp := contentPool.Get().(*[]byte)
			contentCopy := append((*cp)[:0], raw...)
			srcTags := payload.GetTags()
			sp := tagPool.Get().(*[]string)
			tags := append((*sp)[:0], srcTags...)
			bat.AddLog(capturedLog{
				Content:          contentCopy,
				ContentPoolSlice: cp,
				Status:           payload.GetStatus(),
				Tags:             tags,
				TagPoolSlice:     sp,
				Hostname:         payload.GetHostname(),
				TimestampNs:      time.Now().UnixNano(),
				Source:           "",
			})
		})
	}

	// Subscribe to all trace stats hooks.
	for _, sh := range req.TraceStatsHooks {
		if sh == nil {
			continue
		}
		sh.SubscribeWithBuffer("flightrecorder-trace-stats", hookBufSize, func(payload hook.TraceStatsView) {
			bat.AddTraceStat(capturedTraceStat{
				Service:          payload.GetService(),
				Name:             payload.GetName(),
				Resource:         payload.GetResource(),
				Type:             payload.GetType(),
				SpanKind:         payload.GetSpanKind(),
				HTTPStatusCode:   payload.GetHTTPStatusCode(),
				Hits:             payload.GetHits(),
				Errors:           payload.GetErrors(),
				DurationNs:       payload.GetDuration(),
				TopLevelHits:     payload.GetTopLevelHits(),
				OkSummary:        payload.GetOkSummary(),
				ErrorSummary:     payload.GetErrorSummary(),
				Hostname:         payload.GetHostname(),
				Env:              payload.GetEnv(),
				Version:          payload.GetVersion(),
				BucketStartNs:    int64(payload.GetBucketStartNs()),
				BucketDurationNs: int64(payload.GetBucketDurationNs()),
				TimestampNs:      time.Now().UnixNano(),
			})
		})
	}

	req.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			bat.Stop()
			return transport.Close()
		},
	})

	pkglog.Infof("flightrecorder: started (socket=%s flush=%s pts=%d defs=%d logs=%d tss=%d hook_buffer=%d ctx_cap=%d)",
		socketPath, flushInterval, ptCapacity, defCapacity, logCapacity, traceStatsCapacity, hookBufSize, contextCap)

	return Provides{Comp: impl}, nil
}
