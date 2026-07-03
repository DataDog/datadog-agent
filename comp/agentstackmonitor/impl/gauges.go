// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package agentstackmonitorimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

const subsystem = "agentstackmonitor"

// Multiple pods of the same subject_kind collapse to one series: resource
// gauges sum, state gauges are 1 if any container matches. Per-pod detail
// lives in the healthplatform issues, not here.
var (
	subjectLabels       = []string{"subject_kind"}
	subjectReasonLabels = []string{"subject_kind", "reason"}
)

type resourceSums struct {
	cpu, mem, memLimit, restart float64
}

type tickAggregate struct {
	resources     map[string]*resourceSums
	terminated    map[[2]string]struct{}
	waiting       map[[2]string]struct{}
	podTerminated map[[2]string]struct{}
}

func newTickAggregate() *tickAggregate {
	return &tickAggregate{
		resources:     make(map[string]*resourceSums),
		terminated:    make(map[[2]string]struct{}),
		waiting:       make(map[[2]string]struct{}),
		podTerminated: make(map[[2]string]struct{}),
	}
}

func (a *tickAggregate) addResource(s *subjectState) {
	kind := string(s.subjectKind)
	sums, ok := a.resources[kind]
	if !ok {
		sums = &resourceSums{}
		a.resources[kind] = sums
	}
	sums.cpu += s.cpuTotal
	sums.mem += s.memUsage
	sums.memLimit += s.memLimit
	sums.restart += float64(s.lastRestartCount)
}

func (a *tickAggregate) addStatus(s *subjectState) {
	kind := string(s.subjectKind)
	if r := currentWaitingReason(s); r != "" {
		a.waiting[[2]string{kind, r}] = struct{}{}
	}
	if s.lastTermReason != "" {
		a.terminated[[2]string{kind, s.lastTermReason}] = struct{}{}
	}
}

func (a *tickAggregate) addPodStatus(s *subjectState) {
	if s.podReason != "" {
		a.podTerminated[[2]string{string(s.subjectKind), s.podReason}] = struct{}{}
	}
}

type gauges struct {
	cpuUsage      telemetry.Gauge
	memoryUsage   telemetry.Gauge
	memoryLimit   telemetry.Gauge
	restartCount  telemetry.Gauge
	terminated    telemetry.Gauge
	waiting       telemetry.Gauge
	podTerminated telemetry.Gauge

	// Label combinations written last tick, kept so stale entries can be
	// deleted before the next set.
	lastResources     map[string]struct{}
	lastTerminated    map[[2]string]struct{}
	lastWaiting       map[[2]string]struct{}
	lastPodTerminated map[[2]string]struct{}
}

func newGauges(t telemetry.Component) *gauges {
	return &gauges{
		cpuUsage:          t.NewGauge(subsystem, "cpu_usage", subjectLabels, "Cumulative CPU usage (seconds) summed across all containers of a Datadog-agent-stack subject kind."),
		memoryUsage:       t.NewGauge(subsystem, "memory_usage", subjectLabels, "Memory usage (bytes) summed across all containers of a Datadog-agent-stack subject kind."),
		memoryLimit:       t.NewGauge(subsystem, "memory_limit", subjectLabels, "Memory limit (bytes) summed across all containers of a Datadog-agent-stack subject kind."),
		restartCount:      t.NewGauge(subsystem, "restart_count", subjectLabels, "Cumulative restart count summed across all containers of a Datadog-agent-stack subject kind."),
		terminated:        t.NewGauge(subsystem, "terminated", subjectReasonLabels, "1 if any container of the given subject kind is currently terminated with the given reason, 0 otherwise."),
		waiting:           t.NewGauge(subsystem, "waiting", subjectReasonLabels, "1 if any container of the given subject kind is currently waiting with the given reason, 0 otherwise."),
		podTerminated:     t.NewGauge(subsystem, "pod_terminated", subjectReasonLabels, "1 if any pod of the given subject kind is currently in a terminal state with the given reason, 0 otherwise."),
		lastResources:     make(map[string]struct{}),
		lastTerminated:    make(map[[2]string]struct{}),
		lastWaiting:       make(map[[2]string]struct{}),
		lastPodTerminated: make(map[[2]string]struct{}),
	}
}

// commit deletes any label combos from the previous tick that are missing
// from a, then writes the current tick's values.
func (g *gauges) commit(a *tickAggregate) {
	for kind := range g.lastResources {
		if _, live := a.resources[kind]; !live {
			g.cpuUsage.Delete(kind)
			g.memoryUsage.Delete(kind)
			g.memoryLimit.Delete(kind)
			g.restartCount.Delete(kind)
		}
	}
	newResources := make(map[string]struct{}, len(a.resources))
	for kind, sums := range a.resources {
		g.cpuUsage.Set(sums.cpu, kind)
		g.memoryUsage.Set(sums.mem, kind)
		g.memoryLimit.Set(sums.memLimit, kind)
		g.restartCount.Set(sums.restart, kind)
		newResources[kind] = struct{}{}
	}
	g.lastResources = newResources

	g.lastTerminated = diffAndSet(g.terminated, g.lastTerminated, a.terminated)
	g.lastWaiting = diffAndSet(g.waiting, g.lastWaiting, a.waiting)
	g.lastPodTerminated = diffAndSet(g.podTerminated, g.lastPodTerminated, a.podTerminated)
}

func diffAndSet(g telemetry.Gauge, last, current map[[2]string]struct{}) map[[2]string]struct{} {
	for tags := range last {
		if _, live := current[tags]; !live {
			g.Delete(tags[0], tags[1])
		}
	}
	next := make(map[[2]string]struct{}, len(current))
	for tags := range current {
		g.Set(1, tags[0], tags[1])
		next[tags] = struct{}{}
	}
	return next
}

func currentWaitingReason(s *subjectState) string {
	if s.waitingReason.filled == 0 {
		return ""
	}
	prev := (s.waitingReason.idx + bufferSize - 1) % bufferSize
	return s.waitingReason.buf[prev]
}
