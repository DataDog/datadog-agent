// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipelinesinkimpl

import (
	"context"
	"sync"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	pipelinesink "github.com/DataDog/datadog-agent/comp/pipelinesink/def"
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

// Requires defines the dependencies injected into the pipeline sink component.
type Requires struct {
	Lc     compdef.Lifecycle
	Config config.Component

	MetricsHooks []hook.Hook[observer.MetricView] `group:"hook"`
	LogsHooks    []hook.Hook[observer.LogView]    `group:"hook"`
}

// Provides defines what the component exposes to the fx graph.
type Provides struct {
	Comp pipelinesink.Component
}

// sinkImpl implements pipelinesink.Component.
type sinkImpl struct {
	counters *counters
}

func (s *sinkImpl) Stats() pipelinesink.Stats {
	return s.counters.stats()
}

// noopSink satisfies the Component interface when pipelinesink is disabled.
type noopSink struct{}

func (noopSink) Stats() pipelinesink.Stats { return pipelinesink.Stats{} }

// NewComponent creates the pipeline sink fx component.
func NewComponent(req Requires) (Provides, error) {
	if !req.Config.GetBool("pipelinesink.enabled") {
		pkglog.Debug("pipelinesink: disabled (pipelinesink.enabled=false)")
		return Provides{Comp: noopSink{}}, nil
	}

	socketPath := req.Config.GetString("pipelinesink.socket_path")
	flushInterval := req.Config.GetDuration("pipelinesink.flush_interval")
	if flushInterval <= 0 {
		flushInterval = 100 * time.Millisecond
	}
	bufCapacity := req.Config.GetInt("pipelinesink.buffer_capacity")
	if bufCapacity <= 0 {
		bufCapacity = 5000
	}
	hookBufSize := req.Config.GetInt("pipelinesink.hook_buffer_size")
	if hookBufSize <= 0 {
		hookBufSize = 4096
	}

	c := &counters{}
	transport := newUnixTransport(socketPath, func() { c.incReconnects() })
	bat := newBatcher(transport, flushInterval, bufCapacity, c)

	impl := &sinkImpl{counters: c}

	// Subscribe to all metric hooks.
	for _, mh := range fxutil.GetAndFilterGroup(req.MetricsHooks) {
		mh.SubscribeWithBuffer("pipelinesink-metrics", hookBufSize, func(payload observer.MetricView) {
			raw := payload.GetRawTags()
			sp := tagPool.Get().(*[]string)
			tags := append((*sp)[:0], raw...)
			bat.AddMetric(capturedMetric{
				Name:        payload.GetName(),
				Value:       payload.GetValue(),
				Tags:        tags,
				TagPoolSlice: sp,
				TimestampNs: int64(payload.GetTimestamp() * float64(time.Second/time.Nanosecond)),
				SampleRate:  payload.GetSampleRate(),
				Source:      "",
			})
		})
	}

	// Subscribe to all log hooks.
	for _, lh := range fxutil.GetAndFilterGroup(req.LogsHooks) {
		lh.SubscribeWithBuffer("pipelinesink-logs", hookBufSize, func(payload observer.LogView) {
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

	pkglog.Infof("pipelinesink: started (socket=%s flush=%s capacity=%d hook_buffer=%d)",
		socketPath, flushInterval, bufCapacity, hookBufSize)

	return Provides{Comp: impl}, nil
}
