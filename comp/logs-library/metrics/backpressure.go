// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"sort"
	"sync"
	"time"

	"github.com/benbjohnson/clock"
)

const (
	// BackpressureSaturated means a component is at or above threshold right now.
	BackpressureSaturated = "SATURATED"
	// BackpressureWarning means a component was saturated in the trailing 30m, but not now.
	BackpressureWarning = "WARNING"
	// BackpressureHealthy means no component has been saturated in the trailing 30m.
	BackpressureHealthy = "HEALTHY"
)

// NoBottleneck labels a loss recorded while the pipeline was healthy.
const NoBottleneck = "none"

// A read fresher than the utilization sampler's interval cannot contain new information.
const bottleneckCacheTTL = utilizationSampleInterval

// ComponentBackpressure is one pipeline component's saturation.
type ComponentBackpressure struct {
	Component           string  `json:"component"`
	Instance            string  `json:"instance"`
	AvgRatio            float64 `json:"avg_ratio"`
	Max5m               float64 `json:"max_5m"`
	Max30m              float64 `json:"max_30m"`
	Max2h               float64 `json:"max_2h"`
	Max5h               float64 `json:"max_5h"`
	Max10h              float64 `json:"max_10h"`
	Saturated1mSeconds  int64   `json:"saturated_1m_s"`
	Saturated30mSeconds int64   `json:"saturated_30m_s"`
	CurrentlySaturated  bool    `json:"currently_saturated"`
	// Used only to correlate saturation with the post-rotation read window. These fields
	// are intentionally not part of the health-platform wire representation.
	LastSaturatedAt  time.Time `json:"-"`
	HasLastSaturated bool      `json:"-"`
}

// BackpressureSummary is the whole pipeline's saturation at one instant.
type BackpressureSummary struct {
	State string `json:"state"`
	// Bottleneck is nil when State is HEALTHY.
	Bottleneck *ComponentBackpressure  `json:"bottleneck"`
	Components []ComponentBackpressure `json:"components"`
}

// outranks reports whether candidate beats incumbent on value, breaking ties on component
// then instance. Snapshots arrive in map order, so ties must not depend on it.
func outranks[T int64 | float64](candidate, incumbent *ComponentBackpressure, candidateVal, incumbentVal T) bool {
	if candidateVal != incumbentVal {
		return candidateVal > incumbentVal
	}
	if candidate.Component != incumbent.Component {
		return candidate.Component < incumbent.Component
	}
	return candidate.Instance < incumbent.Instance
}

// SelectBottleneck returns the overall state and the component responsible for it. Callers
// filter their own input; nothing is excluded here.
func SelectBottleneck(comps []ComponentBackpressure) (string, *ComponentBackpressure) {
	var currSat, sat1m, sat30m *ComponentBackpressure

	for i := range comps {
		c := &comps[i]
		if c.CurrentlySaturated && (currSat == nil || outranks(c, currSat, c.AvgRatio, currSat.AvgRatio)) {
			currSat = c
		}
		if c.Saturated1mSeconds > 0 && (sat1m == nil || outranks(c, sat1m, c.Saturated1mSeconds, sat1m.Saturated1mSeconds)) {
			sat1m = c
		}
		if c.Saturated30mSeconds > 0 && (sat30m == nil || outranks(c, sat30m, c.Saturated30mSeconds, sat30m.Saturated30mSeconds)) {
			sat30m = c
		}
	}

	switch {
	case currSat != nil:
		return BackpressureSaturated, currSat
	case sat1m != nil:
		return BackpressureWarning, sat1m
	case sat30m != nil:
		return BackpressureWarning, sat30m
	}
	return BackpressureHealthy, nil
}

// DeriveBackpressure summarises a pipeline monitor's snapshots. A monitor with no measurable
// component yields the zero value: measuring nothing is not measuring a healthy pipeline.
func DeriveBackpressure(snaps []ComponentSnapshot) BackpressureSummary {
	comps := make([]ComponentBackpressure, 0, len(snaps))
	for _, s := range snaps {
		// "sender" has no utilization monitor, so its ratio is always 0.
		if s.Name == SenderTlmName {
			continue
		}
		comps = append(comps, ComponentBackpressure{
			Component:           s.Name,
			Instance:            s.Instance,
			AvgRatio:            s.AvgRatio,
			Max5m:               s.Windows.Max5m,
			Max30m:              s.Windows.Max30m,
			Max2h:               s.Windows.Max2h,
			Max5h:               s.Windows.Max5h,
			Max10h:              s.Windows.Max10h,
			Saturated1mSeconds:  int64(s.Windows.Saturated1m.Seconds()),
			Saturated30mSeconds: int64(s.Windows.Saturated30m.Seconds()),
			CurrentlySaturated:  s.Windows.CurrentlySaturated,
			LastSaturatedAt:     s.Windows.LastSaturatedAt,
			HasLastSaturated:    s.Windows.HasLastSaturated,
		})
	}

	if len(comps) == 0 {
		return BackpressureSummary{}
	}

	state, bottleneck := SelectBottleneck(comps)
	summary := BackpressureSummary{State: state, Components: comps}
	if bottleneck != nil {
		// Copy: the sort below moves the element the pointer refers to.
		b := *bottleneck
		summary.Bottleneck = &b
	}

	// Bottleneck first: it is picked on recency and sorted on duration, so it does not
	// otherwise survive a caller that truncates.
	isBottleneck := func(c *ComponentBackpressure) bool {
		return summary.Bottleneck != nil &&
			c.Component == summary.Bottleneck.Component &&
			c.Instance == summary.Bottleneck.Instance
	}

	sort.Slice(comps, func(i, j int) bool {
		if bi, bj := isBottleneck(&comps[i]), isBottleneck(&comps[j]); bi != bj {
			return bi
		}
		if comps[i].Saturated30mSeconds != comps[j].Saturated30mSeconds {
			return comps[i].Saturated30mSeconds > comps[j].Saturated30mSeconds
		}
		if comps[i].AvgRatio != comps[j].AvgRatio {
			return comps[i].AvgRatio > comps[j].AvgRatio
		}
		if comps[i].Component != comps[j].Component {
			return comps[i].Component < comps[j].Component
		}
		return comps[i].Instance < comps[j].Instance
	})

	return summary
}

// Process-wide because runner.HealthCheckFunc takes no arguments.
var registeredMonitor struct {
	sync.RWMutex
	pm PipelineMonitor
}

// RegisterPipelineMonitor records the monitor whose snapshots process-wide readers see.
func RegisterPipelineMonitor(pm PipelineMonitor) {
	registeredMonitor.Lock()
	defer registeredMonitor.Unlock()
	registeredMonitor.pm = pm
	// A new pipeline invalidates the old one's bottleneck.
	bottleneck.invalidate()
}

func registeredPipelineMonitor() PipelineMonitor {
	registeredMonitor.RLock()
	defer registeredMonitor.RUnlock()
	return registeredMonitor.pm
}

// BackpressureSnapshot summarises the registered pipeline monitor, or returns the zero value
// when none is registered. Callers must read that as "unknown", not as healthy.
func BackpressureSnapshot() BackpressureSummary {
	pm := registeredPipelineMonitor()
	if pm == nil {
		return BackpressureSummary{}
	}
	return DeriveBackpressure(pm.Snapshots())
}

// bottleneckCache memoizes the derived summary: reading it walks every component's rolling
// history, while correlating that immutable summary with each rotation window is cheap.
type bottleneckCache struct {
	mu         sync.Mutex
	clk        clock.Clock
	summary    BackpressureSummary
	readAt     time.Time
	generation uint64
	valid      bool
}

func newBottleneckCache(clk clock.Clock) *bottleneckCache {
	return &bottleneckCache{clk: clk}
}

func (c *bottleneckCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	c.valid = false
}

// bottleneckDuringLoss names a component that saturated during the actual post-rotation read
// window. A recovered component is eligible only when its last saturated sample is at or after
// the rotation; saturation before the rotation cannot have caused this loss.
func bottleneckDuringLoss(summary BackpressureSummary, lossWindowStartedAt, now time.Time) string {
	if summary.State == "" || lossWindowStartedAt.IsZero() || lossWindowStartedAt.After(now) {
		return ""
	}

	var current, recovered *ComponentBackpressure
	for i := range summary.Components {
		component := &summary.Components[i]
		// CurrentlySaturated is debounced, so it can stay true briefly after recovery. When a
		// precise sample timestamp is available, it must still fall inside the loss window.
		saturatedDuringLoss := component.HasLastSaturated &&
			!component.LastSaturatedAt.Before(lossWindowStartedAt)
		switch {
		case component.CurrentlySaturated && (saturatedDuringLoss || !component.HasLastSaturated):
			if current == nil || outranks(component, current, component.AvgRatio, current.AvgRatio) {
				current = component
			}
		case saturatedDuringLoss:
			if recovered == nil || component.LastSaturatedAt.After(recovered.LastSaturatedAt) ||
				(component.LastSaturatedAt.Equal(recovered.LastSaturatedAt) &&
					outranks(component, recovered, component.Saturated30mSeconds, recovered.Saturated30mSeconds)) {
				recovered = component
			}
		}
	}

	if current != nil {
		return current.Component
	}
	if recovered != nil {
		return recovered.Component
	}

	// Saturation durations cover the trailing 30 minutes. Inside that observation window, a
	// healthy summary proves that no measured component saturated after the rotation. For a
	// longer close_timeout, absence of a timestamp is not enough evidence, so stay unknown.
	if summary.State == BackpressureHealthy && now.Sub(lossWindowStartedAt) <= 30*time.Minute {
		return NoBottleneck
	}
	return ""
}

// get returns the bottleneck's component name without its instance, bounding cardinality.
func (c *bottleneckCache) get(lossWindowStartedAt time.Time) string {
	for {
		now := c.clk.Now()

		c.mu.Lock()
		generation := c.generation
		if c.valid && now.Sub(c.readAt) < bottleneckCacheTTL && !c.readAt.Before(lossWindowStartedAt) {
			summary := c.summary
			c.mu.Unlock()
			return bottleneckDuringLoss(summary, lossWindowStartedAt, now)
		}
		c.mu.Unlock()

		// Derived outside the lock: a rotating tailer must not block on another tailer's read.
		summary := BackpressureSnapshot()

		c.mu.Lock()
		if generation != c.generation {
			// The registered pipeline changed while its snapshot was being derived. Retry so
			// neither this caller nor the cache observes the stopped pipeline.
			c.mu.Unlock()
			continue
		}
		c.summary = summary
		c.readAt = now
		c.valid = true
		c.mu.Unlock()

		return bottleneckDuringLoss(summary, lossWindowStartedAt, now)
	}
}

var bottleneck = newBottleneckCache(clock.New())

// currentBottleneckComponent names a stage saturated during the loss window, NoBottleneck
// when the pipeline was measured as healthy throughout it, or "" when attribution is unknown.
func currentBottleneckComponent(lossWindowStartedAt time.Time) string {
	return bottleneck.get(lossWindowStartedAt)
}
