// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"testing"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// stubMetric is a minimal MetricView for tests.
type stubMetric struct{}

func (stubMetric) GetName() string         { return "test.metric" }
func (stubMetric) GetValue() float64       { return 1.0 }
func (stubMetric) GetRawTags() []string    { return []string{"env:test"} }
func (stubMetric) GetTimestampUnix() int64 { return 1000 }
func (stubMetric) GetSampleRate() float64  { return 1.0 }

// stubLog is a minimal LogView for tests.
type stubLog struct{}

func (stubLog) GetContent() []byte           { return []byte("hello") }
func (stubLog) GetStatus() string            { return "info" }
func (stubLog) GetTags() []string            { return []string{"service:test"} }
func (stubLog) GetHostname() string          { return "host1" }
func (stubLog) GetTimestampUnixMilli() int64 { return 1000000 }

// newTestObserver creates an observerImpl without fx for unit tests.
func newTestObserver() *observerImpl {
	obs := &observerImpl{
		obsCh: make(chan observation, 100),
	}
	obs.handleFunc = obs.innerHandle
	go obs.run()
	return obs
}

func TestGetHandleNotNil(t *testing.T) {
	obs := newTestObserver()
	h := obs.GetHandle("test-source")
	if h == nil {
		t.Fatal("GetHandle returned nil")
	}
}

func TestObserveMetricNoPanic(t *testing.T) {
	obs := newTestObserver()
	h := obs.GetHandle("test-source")
	h.ObserveMetric(stubMetric{})
}

func TestObserveLogNoPanic(t *testing.T) {
	obs := newTestObserver()
	h := obs.GetHandle("test-source")
	h.ObserveLog(stubLog{})
}

func TestHandleDropsWhenFull(t *testing.T) {
	// Do NOT start run() so the channel fills up.
	obs := &observerImpl{
		obsCh: make(chan observation, 2),
	}
	obs.handleFunc = obs.innerHandle

	h := obs.GetHandle("overload")
	h.ObserveMetric(stubMetric{})
	h.ObserveMetric(stubMetric{})
	// Channel full — these must not block.
	h.ObserveMetric(stubMetric{})
	h.ObserveMetric(stubMetric{})

	if h.(*handle).dropCount.Load() < 2 {
		t.Errorf("expected at least 2 drops, got %d", h.(*handle).dropCount.Load())
	}
}

func TestNoopHandleDiscards(t *testing.T) {
	obs := newTestObserver()
	obs.handleFunc = obs.noopHandle
	h := obs.GetHandle("noop-source")

	if _, ok := h.(*noopObserveHandle); !ok {
		t.Fatalf("expected *noopObserveHandle, got %T", h)
	}
	h.ObserveMetric(stubMetric{})
	h.ObserveLog(stubLog{})
}

func TestCopyTagsAndBytes(t *testing.T) {
	tags := []string{"a", "b"}
	copied := copyTags(tags)
	tags[0] = "mutated"
	if copied[0] != "a" {
		t.Error("copyTags did not deep-copy")
	}

	b := []byte("hello")
	cb := copyBytes(b)
	b[0] = 'X'
	if cb[0] != 'h' {
		t.Error("copyBytes did not deep-copy")
	}
}

// Ensure observerImpl satisfies the Component interface at compile time.
var _ observerdef.Component = (*observerImpl)(nil)
