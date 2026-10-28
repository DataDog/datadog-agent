// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodeunschedulable

import (
	"errors"
	"slices"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config/env"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// ReasonUnschedulable is reported while the node is unschedulable.
	ReasonUnschedulable = "unschedulable"

	// TagKey is the tag key of every reason.
	TagKey = "unschedulable_reason"

	// ClosingWindow is how long a node-level series keeps reporting 0 after it stops
	// being the current one, so that a consumer sees an explicit recovery.
	ClosingWindow = 10 * time.Minute
)

var (
	// unsupportedLog and failedReadLog limit the logs of a state that is checked again
	// on every refresh: the first time, then once per interval.
	unsupportedLog = log.NewLogLimit(1, time.Hour)
	failedReadLog  = log.NewLogLimit(1, 10*time.Minute)

	// ErrUnsupported is returned by a Source that cannot provide the state at all
	// (when the cluster agent is disabled).
	ErrUnsupported = errors.New("node unschedulable state not supported")
	// ErrRejected is returned on HTTP 403 or 404: an older cluster agent, or a wrong
	// token, which Probe tells apart.
	ErrRejected = errors.New("node unschedulable request rejected")
)

// Result is the unschedulable state of this node.
type Result struct {
	Unschedulable bool
	// Reasons are the reasons of the matching rules, sorted.
	Reasons []string
}

// Source reads the state of this node.
type Source interface {
	// NodeUnschedulable reads the state of this node. A refused request is
	// ErrRejected, an unusable source ErrUnsupported.
	NodeUnschedulable() (Result, error)
	// Probe calls an endpoint every cluster agent serves; nil means our requests are accepted.
	Probe() error
}

// Outcome is the node-level result of one refresh. An empty Reason means nothing to
// report.
type Outcome struct {
	Reason string
	// Tags are the unschedulable_reason:<reason> tags, sorted.
	Tags []string
}

// Tracker keeps the last state read; a failed read keeps it. Not safe for concurrent use.
type Tracker struct {
	refreshInterval time.Duration
	lastRefresh     time.Time
	last            *Result
}

// NewTracker creates a tracker that reads the state at most once per refresh interval.
func NewTracker(refreshInterval time.Duration) *Tracker {
	if refreshInterval <= 0 {
		refreshInterval = time.Minute
	}
	return &Tracker{refreshInterval: refreshInterval}
}

// Refresh reads the state at most once per refresh interval. An unsupported source
// clears it; any other failure keeps it.
func (t *Tracker) Refresh(src Source, now time.Time) {
	if !t.lastRefresh.IsZero() && now.Sub(t.lastRefresh) < t.refreshInterval {
		return
	}
	t.lastRefresh = now

	res, err := src.NodeUnschedulable()
	switch {
	case err == nil:
		t.last = &res
		return
	case errors.Is(err, ErrUnsupported):
		t.last = nil
		if unsupportedLog.ShouldLog() {
			log.Infof("Unschedulable GPU nodes are not reported: %v", err)
		}
		return
	case errors.Is(err, ErrRejected):
		// An older cluster agent still accepts the probe; a token problem does not.
		if src.Probe() == nil {
			t.last = nil
			if unsupportedLog.ShouldLog() {
				log.Infof("Unschedulable GPU nodes are not reported: the cluster agent does not serve them, it may be older than the node agent (%v)", err)
			}
			return
		}
	}
	if failedReadLog.ShouldLog() {
		log.Debugf("Could not read the unschedulable state of the node, keeping the last known state: %v", err)
	}
}

// Outcome returns what to report: unschedulable with its reasons, or nothing.
func (t *Tracker) Outcome() Outcome {
	switch {
	case t.last == nil:
		return Outcome{}
	case t.last.Unschedulable:
		tags := make([]string, 0, len(t.last.Reasons))
		for _, r := range t.last.Reasons {
			tags = append(tags, TagKey+":"+r)
		}
		slices.Sort(tags)
		return Outcome{Reason: ReasonUnschedulable, Tags: slices.Compact(tags)}
	default:
		return Outcome{}
	}
}

// Point is one value to emit for every device of the node, on top of the device
// tags.
type Point struct {
	Tags  []string
	Value float64
}

type retiredTags struct {
	tags  []string
	since time.Time
}

// Series emits 1 on the current tagset and 0 for ClosingWindow on every tagset that
// stopped being current.
type Series struct {
	open    []string
	retired []retiredTags
}

// Step advances the series to now and returns the points to emit.
func (s *Series) Step(o Outcome, now time.Time) []Point {
	var cur []string
	if o.Reason != "" {
		cur = append([]string{"unavailable_reason:" + o.Reason}, o.Tags...)
		slices.Sort(cur)
		cur = slices.Compact(cur)
	}
	if !slices.Equal(cur, s.open) {
		if s.open != nil {
			s.retire(s.open, now)
		}
		s.open = cur
	}
	if s.open != nil {
		s.cancel(s.open)
	}
	s.retired = slices.DeleteFunc(s.retired, func(r retiredTags) bool { return now.Sub(r.since) >= ClosingWindow })

	var points []Point
	if s.open != nil {
		points = append(points, Point{Tags: s.open, Value: 1})
	}
	for _, r := range s.retired {
		points = append(points, Point{Tags: r.tags, Value: 0})
	}
	return points
}

func (s *Series) retire(tags []string, now time.Time) {
	s.cancel(tags)
	s.retired = append(s.retired, retiredTags{tags: tags, since: now})
}

func (s *Series) cancel(tags []string) {
	s.retired = slices.DeleteFunc(s.retired, func(r retiredTags) bool { return slices.Equal(r.tags, tags) })
}

// Cache holds the points of the current check run. The GPU check refreshes it once
// per run, then every device collector reads the same points.
type Cache struct {
	source  Source
	tracker *Tracker
	series  Series
	points  []Point
}

// NewCache creates a cache that reads src at most once per refresh interval.
func NewCache(src Source, refreshInterval time.Duration) *Cache {
	return &Cache{source: src, tracker: NewTracker(refreshInterval)}
}

// Refresh reads the state if due and steps the series to now. Off Kubernetes there
// is nothing to read.
func (c *Cache) Refresh(now time.Time) {
	if !env.IsFeaturePresent(env.Kubernetes) {
		return
	}
	c.tracker.Refresh(c.source, now)
	c.points = c.series.Step(c.tracker.Outcome(), now)
}

// Points returns the points of the last refresh. Callers must not modify them.
func (c *Cache) Points() []Point {
	return c.points
}
