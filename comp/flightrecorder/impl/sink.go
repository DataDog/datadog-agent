// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorderimpl

import (
	"context"
	"sync"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	flightrecorder "github.com/DataDog/datadog-agent/comp/flightrecorder/def"
	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/DataDog/datadog-agent/pkg/hook"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

	MetricsHooks []hook.Hook[observer.MetricView] `group:"hook"`
	LogsHooks    []hook.Hook[observer.LogView]    `group:"hook"`
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
	hookBufSize := req.Config.GetInt("flightrecorder.hook_buffer_size")
	if hookBufSize <= 0 {
		hookBufSize = 4096
	}

	c := &counters{}
	// Create transport and batcher. The reconnect callback is set after
	// batcher creation to avoid a circular dependency.
	transport := newUnixTransport(socketPath, func() { c.incReconnects() })
	bat := newBatcher(transport, flushInterval, ptCapacity, defCapacity, logCapacity, c)
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
	for _, mh := range fxutil.GetAndFilterGroup(req.MetricsHooks) {
		source := mh.Name()
		mh.SubscribeWithBuffer("flightrecorder-metrics", hookBufSize, func(payload observer.MetricView) {
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
	for _, lh := range fxutil.GetAndFilterGroup(req.LogsHooks) {
		lh.SubscribeWithBuffer("flightrecorder-logs", hookBufSize, func(payload observer.LogView) {
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

	req.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			bat.Stop()
			return transport.Close()
		},
	})

	pkglog.Infof("flightrecorder: started (socket=%s flush=%s pts=%d defs=%d logs=%d hook_buffer=%d)",
		socketPath, flushInterval, ptCapacity, defCapacity, logCapacity, hookBufSize)

	return Provides{Comp: impl}, nil
}
