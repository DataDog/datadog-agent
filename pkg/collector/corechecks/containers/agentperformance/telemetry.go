// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentperformance records Agent performance COAT metrics from container checks.
package agentperformance

import (
	"math"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	subsystem = "agent_performance"
	kindTag   = "kind"

	// ContainerRestarts is the metric name for Kubernetes container restarts.
	ContainerRestarts = "container_restarts"
	// ContainerTerminated is the metric name for Kubernetes container terminated states.
	ContainerTerminated = "container_terminated"
	// MemoryUsage is the metric name for container runtime memory usage.
	MemoryUsage = "memory_usage"
	// MemoryLimit is the metric name for container runtime memory limits.
	MemoryLimit = "memory_limit"
	// CPUUsage is the metric name for container runtime CPU cores used.
	CPUUsage = "cpu_usage"

	nodeAgentComponent                  = "agent"
	datadogComponentLabelKey            = "agent.datadoghq.com/component"
	clusterAgentComponent               = "cluster-agent"
	clusterChecksAgentComponentHelm     = "clusterchecks-agent"
	clusterChecksAgentComponentOperator = "cluster-checks-runner"
)

var (
	defaultRecorder     *Recorder
	defaultRecorderOnce sync.Once
)

// Recorder records COAT metrics for Datadog Agent pods.
type Recorder struct {
	containersRestarts   telemetry.Gauge
	containersTerminated telemetry.Gauge
	memoryUsage          telemetry.Gauge
	memoryLimits         telemetry.Gauge
	cpuUsage             telemetry.Gauge

	runtimeSnapshotMu  sync.Mutex
	previousCPUSamples map[string]cpuSample
	currentCPUSamples  map[string]cpuSample
}

type cpuSample struct {
	total     float64
	timestamp time.Time
}

// NewRecorder returns the shared COAT recorder for Datadog Agent pods.
func NewRecorder(tm telemetry.Component) *Recorder {
	defaultRecorderOnce.Do(func() {
		defaultRecorder = newRecorder(tm)
	})
	return defaultRecorder
}

func newRecorder(tm telemetry.Component) *Recorder {
	return &Recorder{
		containersRestarts: tm.NewGauge(
			subsystem,
			ContainerRestarts,
			[]string{kindTag, tags.KubePod},
			"Sum of kubernetes.containers.restarts for Datadog Agent pods",
		),
		containersTerminated: tm.NewGauge(
			subsystem,
			ContainerTerminated,
			[]string{kindTag, tags.KubePod, "reason"},
			"Sum of kubernetes.containers.*.terminated for Datadog Agent pods",
		),
		memoryUsage: tm.NewGauge(
			subsystem,
			MemoryUsage,
			[]string{kindTag, tags.KubePod},
			"Sum of container runtime memory usage for Datadog Agent pods",
		),
		memoryLimits: tm.NewGauge(
			subsystem,
			MemoryLimit,
			[]string{kindTag, tags.KubePod},
			"Sum of container runtime memory limits for Datadog Agent pods",
		),
		cpuUsage: tm.NewGauge(
			subsystem,
			CPUUsage,
			[]string{kindTag, tags.KubePod},
			"Sum of CPU cores used by Datadog Agent pod containers, derived from cumulative CPU time",
		),
		previousCPUSamples: make(map[string]cpuSample),
		currentCPUSamples:  make(map[string]cpuSample),
	}
}

// WithRuntimeMetrics runs callback within an exclusive runtime metrics snapshot transaction.
func (t *Recorder) WithRuntimeMetrics(callback func() error) error {
	t.runtimeSnapshotMu.Lock()
	defer t.runtimeSnapshotMu.Unlock()

	t.resetRuntimeMetrics()
	defer t.completeRuntimeMetrics()

	return callback()
}

func (t *Recorder) completeRuntimeMetrics() {
	t.previousCPUSamples, t.currentCPUSamples = t.currentCPUSamples, t.previousCPUSamples
}

// MarkCPUContainerPresent retains the prior CPU sample for a listed container without a new sample.
// It must be called from a WithRuntimeMetrics callback.
func (t *Recorder) MarkCPUContainerPresent(containerID string) {
	if containerID == "" {
		return
	}

	if previousSample, ok := t.previousCPUSamples[containerID]; ok {
		t.currentCPUSamples[containerID] = previousSample
	}
}

// RecordCPUUsage adds a container's delta-derived CPU cores to its eligible Agent pod aggregate.
// It must be called from a WithRuntimeMetrics callback.
func (t *Recorder) RecordCPUUsage(containerID string, total *float64, timestamp time.Time, pod *workloadmeta.KubernetesPod) {
	t.MarkCPUContainerPresent(containerID)
	if containerID == "" || total == nil {
		return
	}

	if *total < 0 || math.IsNaN(*total) || math.IsInf(*total, 0) {
		return
	}

	currentSample := cpuSample{total: *total, timestamp: timestamp}
	t.currentCPUSamples[containerID] = currentSample

	previousSample, ok := t.previousCPUSamples[containerID]
	if !ok || timestamp.IsZero() || previousSample.timestamp.IsZero() || *total <= previousSample.total {
		return
	}

	elapsed := timestamp.Sub(previousSample.timestamp)
	if elapsed <= 0 {
		return
	}

	kind, ok := agentPodKind(pod)
	if !ok || pod.Name == "" {
		return
	}

	t.cpuUsage.Add((*total-previousSample.total)/float64(elapsed), kind, pod.Name)
}

// ResetKubeletMetrics clears kubelet-sourced state aggregates.
func (t *Recorder) ResetKubeletMetrics() {
	t.resetKubeletMetrics()
}

// RecordMetric adds a metric to the COAT aggregate when it belongs to
// a Datadog Agent, Cluster Agent, or Cluster Check Runner pod.
func (t *Recorder) RecordMetric(metricName string, value *float64, pod *workloadmeta.KubernetesPod, reason string) {
	if value == nil || pod == nil {
		return
	}

	kind, ok := agentPodKind(pod)
	if !ok {
		return
	}

	if pod.Name == "" {
		return
	}
	t.record(metricName, *value, kind, pod.Name, reason)
}

func (t *Recorder) resetRuntimeMetrics() {
	for _, kind := range []string{nodeAgentComponent, clusterAgentComponent, clusterChecksAgentComponentOperator} {
		match := map[string]string{kindTag: kind}
		t.memoryUsage.DeletePartialMatch(match)
		t.memoryLimits.DeletePartialMatch(match)
		t.cpuUsage.DeletePartialMatch(match)
	}

	clear(t.currentCPUSamples)
}

func (t *Recorder) resetKubeletMetrics() {
	for _, kind := range []string{nodeAgentComponent, clusterAgentComponent, clusterChecksAgentComponentOperator} {
		match := map[string]string{kindTag: kind}
		t.containersRestarts.DeletePartialMatch(match)
		t.containersTerminated.DeletePartialMatch(match)
	}
}

func (t *Recorder) record(metricName string, value float64, kind string, podName string, reason string) {
	switch metricName {
	case ContainerRestarts:
		t.containersRestarts.Add(value, kind, podName)
	case ContainerTerminated:
		if reason == "" {
			return
		}
		t.containersTerminated.Add(value, kind, podName, reason)
	case MemoryUsage:
		t.memoryUsage.Add(value, kind, podName)
	case MemoryLimit:
		t.memoryLimits.Add(value, kind, podName)
	}
}

func agentPodKind(pod *workloadmeta.KubernetesPod) (string, bool) {
	if pod == nil {
		return "", false
	}
	switch component := pod.Labels[kubernetes.KubeAppComponentLabelKey]; component {
	case nodeAgentComponent:
		// The generic Kubernetes component label is not a Datadog identity. Node Agent
		// manifests set this vendor-specific label too, preventing unrelated workloads
		// from entering COAT telemetry as Datadog Agent pods.
		if pod.Labels[datadogComponentLabelKey] == nodeAgentComponent {
			return component, true
		}
	case clusterAgentComponent:
		return component, true
	case clusterChecksAgentComponentHelm, clusterChecksAgentComponentOperator:
		// consolidate component name difference between helm and operator
		return clusterChecksAgentComponentOperator, true
	}

	return "", false
}
