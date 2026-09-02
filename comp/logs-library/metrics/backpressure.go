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

// Backpressure states, shared by the status page and the health platform issue.
const (
	// BackpressureSaturated means a component is at or above threshold right now.
	BackpressureSaturated = "SATURATED"
	// BackpressureWarning means a component was saturated in the trailing 30m, but not now.
	BackpressureWarning = "WARNING"
	// BackpressureHealthy means no component has been saturated in the trailing 30m.
	BackpressureHealthy = "HEALTHY"
)

// NoBottleneck labels a loss recorded while the pipeline was healthy: rotation outpaced the
// tailer's close_timeout, not the pipeline's throughput.
const NoBottleneck = "none"

// A read fresher than the utilization sampler's interval cannot contain new information.
const bottleneckCacheTTL = utilizationSampleInterval

// ComponentBackpressure is one pipeline component's saturation. The JSON tags are the wire
// contract with the health platform issue template.
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
}

// BackpressureSummary is the whole pipeline's saturation at one instant.
type BackpressureSummary struct {
	State string `json:"state"`
	// Bottleneck is nil when State is HEALTHY.
	Bottleneck *ComponentBackpressure  `json:"bottleneck"`
	Components []ComponentBackpressure `json:"components"`
}

// SelectBottleneck returns the overall state and the component responsible for it. Callers
// filter their own input; nothing is excluded here.
func SelectBottleneck(comps []ComponentBackpressure) (string, *ComponentBackpressure) {
	// Precedence: saturated now (highest EWMA wins), else the most 1m saturation, else 30m.
	var currSat, sat1m, sat30m *ComponentBackpressure

	for i := range comps {
		c := &comps[i]
		if c.CurrentlySaturated && (currSat == nil || c.AvgRatio > currSat.AvgRatio) {
			currSat = c
		}
		if c.Saturated1mSeconds > 0 && (sat1m == nil || c.Saturated1mSeconds > sat1m.Saturated1mSeconds) {
			sat1m = c
		}
		if c.Saturated30mSeconds > 0 && (sat30m == nil || c.Saturated30mSeconds > sat30m.Saturated30mSeconds) {
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

// DeriveBackpressure summarises a pipeline monitor's snapshots.
func DeriveBackpressure(snaps []ComponentSnapshot) BackpressureSummary {
	comps := make([]ComponentBackpressure, 0, len(snaps))
	for _, s := range snaps {
		// "sender" is a capacity-only aggregation point with no utilization monitor, so its
		// ratio is always 0. Excluded here exactly as on the status page.
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
		})
	}

	state, bottleneck := SelectBottleneck(comps)
	summary := BackpressureSummary{State: state, Components: comps}
	if bottleneck != nil {
		// Copy: the sort below moves the element the pointer refers to.
		b := *bottleneck
		summary.Bottleneck = &b
	}

	// Worst first, so a caller that truncates keeps the saturated rows. Ties break on name so
	// two reads of an unchanged pipeline encode identically.
	sort.Slice(comps, func(i, j int) bool {
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

// Process-wide because runner.HealthCheckFunc takes no arguments. Like missedBytes.
var registeredMonitor struct {
	sync.RWMutex
	pm PipelineMonitor
}

// RegisterPipelineMonitor records the monitor whose snapshots process-wide readers see. Call
// only from the logs agent's pipeline start path, alongside MarkLogsAgentRunning.
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

// bottleneckCache memoizes the bottleneck's component name: deriving it walks every
// component's rolling history, once per rotated file.
type bottleneckCache struct {
	mu        sync.Mutex
	clk       clock.Clock
	component string
	readAt    time.Time
	valid     bool
}

func newBottleneckCache(clk clock.Clock) *bottleneckCache {
	return &bottleneckCache{clk: clk}
}

func (c *bottleneckCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.valid = false
}

// get returns the bottleneck's component name without its instance, which is what the
// remediation branches on and bounds the label's cardinality.
func (c *bottleneckCache) get() string {
	now := c.clk.Now()

	c.mu.Lock()
	if c.valid && now.Sub(c.readAt) < bottleneckCacheTTL {
		component := c.component
		c.mu.Unlock()
		return component
	}
	c.mu.Unlock()

	// Derived outside the lock: blocking a rotating tailer behind another tailer's read would
	// be a new source of backpressure.
	summary := BackpressureSnapshot()
	component := ""
	switch {
	case summary.Bottleneck != nil:
		component = summary.Bottleneck.Component
	case summary.State != "":
		// Distinct from "": a monitor answered, and nothing was saturated.
		component = NoBottleneck
	}

	c.mu.Lock()
	c.component = component
	c.readAt = now
	c.valid = true
	c.mu.Unlock()

	return component
}

var bottleneck = newBottleneckCache(clock.New())

// currentBottleneckComponent names the saturated stage, NoBottleneck when the pipeline is
// healthy, or "" when no monitor is registered.
func currentBottleneckComponent() string {
	return bottleneck.get()
}
