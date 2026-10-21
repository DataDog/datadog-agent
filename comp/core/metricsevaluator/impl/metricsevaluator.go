// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package metricsevaluatorimpl implements the metricsevaluator component interface
package metricsevaluatorimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	metricsevaluator "github.com/DataDog/datadog-agent/comp/core/metricsevaluator/def"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// evaluationInterval is how often every monitor's window is sampled and evaluated.
const evaluationInterval = time.Minute

// Requires defines the dependencies for the metricsevaluator component
type Requires struct {
	Log       log.Component
	Lifecycle compdef.Lifecycle
	Telemetry telemetry.Component
}

// Provides defines the output of the metricsevaluator component
type Provides struct {
	Comp metricsevaluator.Component
}

type evaluator struct {
	log       log.Component
	telemetry telemetry.Component
	interval  time.Duration

	mu       sync.Mutex
	monitors map[string]*monitorState

	stop chan struct{}
	done chan struct{}
}

// NewComponent creates a new metricsevaluator component
func NewComponent(reqs Requires) Provides {
	e := newEvaluator(reqs.Log, reqs.Telemetry, evaluationInterval)

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			go e.run()
			return nil
		},
		OnStop: func(ctx context.Context) error {
			close(e.stop)
			select {
			case <-e.done:
			case <-ctx.Done():
			}
			return nil
		},
	})

	return Provides{Comp: e}
}

func newEvaluator(logger log.Component, telComp telemetry.Component, interval time.Duration) *evaluator {
	return &evaluator{
		log:       logger,
		telemetry: telComp,
		interval:  interval,
		monitors:  make(map[string]*monitorState),
		stop:      make(chan struct{}),
		done:      make(chan struct{}),
	}
}

// Register implements metricsevaluator.Component
func (e *evaluator) Register(m metricsevaluator.Monitor) error {
	if m.OnNoData == "" {
		m.OnNoData = metricsevaluator.NoDataKeepState
	}
	if err := validateMonitor(m, e.interval); err != nil {
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.monitors[m.Name]; ok {
		return fmt.Errorf("monitor %q is already registered", m.Name)
	}
	e.monitors[m.Name] = newMonitorState(m, e.interval)
	return nil
}

// Unregister implements metricsevaluator.Component
func (e *evaluator) Unregister(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.monitors, name)
}

func (e *evaluator) run() {
	defer close(e.done)

	ticker := time.NewTicker(e.interval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			e.evaluate(now)
		case <-e.stop:
			return
		}
	}
}

// evaluate samples every monitor's metric once, then fires the callbacks of the monitors whose state changed.
func (e *evaluator) evaluate(now time.Time) {
	e.mu.Lock()
	if len(e.monitors) == 0 {
		e.mu.Unlock()
		return
	}

	names := make([]string, 0, len(e.monitors))
	for _, ms := range e.monitors {
		names = append(names, ms.monitor.Metric)
		if ms.monitor.DivideBy != "" {
			names = append(names, ms.monitor.DivideBy)
		}
	}
	families, err := e.telemetry.Gather(telemetry.StaticMetricFilter(names...))
	if err != nil {
		e.mu.Unlock()
		e.log.Warnf("metrics evaluator: unable to gather telemetry metrics: %v", err)
		return
	}
	byName := make(map[string]*telemetry.MetricFamily, len(families))
	for _, mf := range families {
		byName[mf.GetName()] = mf
	}

	type notification struct {
		callback func(metricsevaluator.Event)
		event    metricsevaluator.Event
	}
	var notifications []notification
	for _, ms := range e.monitors {
		ms.push(monitorSample(ms.monitor, byName))

		aggregated, hasData := ms.aggregate()
		previous := ms.state
		ms.state = nextState(ms.monitor, previous, aggregated, hasData)
		if ms.state != previous {
			notifications = append(notifications, notification{
				callback: ms.monitor.Callback,
				event: metricsevaluator.Event{
					Monitor:   ms.monitor.Name,
					Value:     aggregated,
					State:     ms.state,
					Previous:  previous,
					Timestamp: now,
				},
			})
		}
	}
	e.mu.Unlock()

	// Callbacks run without the lock so they can call Register or Unregister.
	for _, n := range notifications {
		n.callback(n.event)
	}
}

// nextState computes a monitor's state from its aggregated window value.
// hasData is false when the window does not hold enough samples to be evaluated.
func nextState(m metricsevaluator.Monitor, previous metricsevaluator.State, value float64, hasData bool) metricsevaluator.State {
	if !hasData {
		switch m.OnNoData {
		case metricsevaluator.NoDataNotify:
			return metricsevaluator.StateNoData

		case metricsevaluator.NoDataKeepState:
			return previous
		}
	}

	switch m.Comparator {
	case metricsevaluator.ComparatorAbove:
		if value > m.Threshold {
			return metricsevaluator.StateAlert
		}
	case metricsevaluator.ComparatorBelow:
		if value < m.Threshold {
			return metricsevaluator.StateAlert
		}
	}
	return metricsevaluator.StateOK
}

func validateMonitor(m metricsevaluator.Monitor, interval time.Duration) error {
	switch {
	case m.Name == "":
		return errors.New("monitor name is empty")
	case m.Metric == "":
		return errors.New("monitor metric is empty")
	case m.Window < interval:
		return fmt.Errorf("monitor window %s is shorter than the evaluation interval %s", m.Window, interval)
	case m.Callback == nil:
		return errors.New("monitor callback is nil")
	}
	switch m.Aggregation {
	case metricsevaluator.AggregationAvg, metricsevaluator.AggregationMax, metricsevaluator.AggregationMin, metricsevaluator.AggregationDelta:
	default:
		return fmt.Errorf("unknown aggregation %q", m.Aggregation)
	}
	switch m.Comparator {
	case metricsevaluator.ComparatorAbove, metricsevaluator.ComparatorBelow:
	default:
		return fmt.Errorf("unknown comparator %q", m.Comparator)
	}
	switch m.OnNoData {
	case metricsevaluator.NoDataNotify, metricsevaluator.NoDataKeepState:
	default:
		return fmt.Errorf("unknown no data policy %q", m.OnNoData)
	}
	return nil
}
