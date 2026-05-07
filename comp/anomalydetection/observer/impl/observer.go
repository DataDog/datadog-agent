// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package observerimpl implements the observer component.
package observerimpl

import (
	"context"
	"sync/atomic"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// Requires declares the input types to the observer component constructor.
type Requires struct {
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the observer component.
type Provides struct {
	Comp observerdef.Component
}

// observation is a message sent from handles to the observer.
type observation struct {
	source string
	metric *metricObs
	log    *logObs
}

// metricObs contains copied metric data and implements observerdef.MetricView.
type metricObs struct {
	name      string
	value     float64
	tags      []string
	timestamp int64
}

var _ observerdef.MetricView = (*metricObs)(nil)

func (m *metricObs) GetName() string         { return m.name }
func (m *metricObs) GetValue() float64       { return m.value }
func (m *metricObs) GetRawTags() []string    { return m.tags }
func (m *metricObs) GetTimestampUnix() int64 { return m.timestamp }
func (m *metricObs) GetSampleRate() float64  { return 1.0 }

// logObs contains copied log data and implements observerdef.LogView.
type logObs struct {
	content     []byte
	status      string
	tags        []string
	hostname    string
	timestampMs int64
}

var _ observerdef.LogView = (*logObs)(nil)

func (l *logObs) GetContent() []byte           { return l.content }
func (l *logObs) GetStatus() string            { return l.status }
func (l *logObs) GetTags() []string            { return l.tags }
func (l *logObs) GetHostname() string          { return l.hostname }
func (l *logObs) GetTimestampUnixMilli() int64 { return l.timestampMs }

// observerImpl is the implementation of the observer component.
type observerImpl struct {
	obsCh      chan observation
	handleFunc observerdef.HandleFunc
}

// NewComponent creates an observer.Component.
func NewComponent(deps Requires) Provides {
	obs := &observerImpl{
		obsCh: make(chan observation, 1000),
	}
	obs.handleFunc = obs.innerHandle

	ch := obs.obsCh
	deps.Lifecycle.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			close(ch)
			return nil
		},
	})

	go obs.run()

	return Provides{Comp: obs}
}

// run drains the observation channel. The engine is wired in a later wave;
// until then observations are discarded to keep overhead at zero.
func (o *observerImpl) run() {
	for range o.obsCh {
	}
}

// GetHandle returns a lightweight handle for a named source.
func (o *observerImpl) GetHandle(name string) observerdef.Handle {
	return o.handleFunc(name)
}

// DumpMetrics writes all stored metrics to the specified file (for debugging).
func (o *observerImpl) DumpMetrics(_ string) error {
	return nil
}

// innerHandle creates a channel-backed handle for the given source name.
func (o *observerImpl) innerHandle(name string) observerdef.Handle {
	return &handle{ch: o.obsCh, source: name}
}

// noopHandle returns a handle that discards all observations.
func (o *observerImpl) noopHandle(_ string) observerdef.Handle {
	return &noopObserveHandle{}
}

// handle is the lightweight observation interface passed to other components.
// It holds a channel reference and source name; all heavy processing happens
// in the observer's run loop.
type handle struct {
	ch        chan<- observation
	source    string
	dropCount atomic.Int64
}

var _ observerdef.Handle = (*handle)(nil)

// ObserveMetric observes a DogStatsD metric sample.
func (h *handle) ObserveMetric(sample observerdef.MetricView) {
	timestamp := sample.GetTimestampUnix()
	if timestamp == 0 {
		timestamp = time.Now().Unix()
	}

	obs := observation{
		source: h.source,
		metric: &metricObs{
			name:      sample.GetName(),
			value:     sample.GetValue(),
			tags:      copyTags(sample.GetRawTags()),
			timestamp: timestamp,
		},
	}

	select {
	case h.ch <- obs:
	default:
		h.dropCount.Add(1)
	}
}

// ObserveLog observes a log message.
func (h *handle) ObserveLog(msg observerdef.LogView) {
	timestampMs := msg.GetTimestampUnixMilli()
	if timestampMs == 0 {
		timestampMs = time.Now().UnixMilli()
	}

	obs := observation{
		source: h.source,
		log: &logObs{
			content:     copyBytes(msg.GetContent()),
			status:      msg.GetStatus(),
			tags:        copyTags(msg.GetTags()),
			hostname:    msg.GetHostname(),
			timestampMs: timestampMs,
		},
	}

	select {
	case h.ch <- obs:
	default:
		h.dropCount.Add(1)
	}
}

// noopObserveHandle discards all observations.
type noopObserveHandle struct{}

var _ observerdef.Handle = (*noopObserveHandle)(nil)

func (h *noopObserveHandle) ObserveMetric(_ observerdef.MetricView) {}
func (h *noopObserveHandle) ObserveLog(_ observerdef.LogView)       {}

func copyTags(tags []string) []string {
	if tags == nil {
		return nil
	}
	out := make([]string, len(tags))
	copy(out, tags)
	return out
}

func copyBytes(b []byte) []byte {
	if b == nil {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
