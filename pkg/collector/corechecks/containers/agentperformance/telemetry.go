// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package agentperformance records Agent performance COAT metrics from container checks.
package agentperformance

import (
	"sync"

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
			"Sum of kubernetes.containers.restarts for Datadog Cluster Agent pods",
		),
		containersTerminated: tm.NewGauge(
			subsystem,
			ContainerTerminated,
			[]string{kindTag, tags.KubePod, "reason"},
			"Sum of kubernetes.containers.*.terminated for Datadog Cluster Agent pods",
		),
		memoryUsage: tm.NewGauge(
			subsystem,
			MemoryUsage,
			[]string{kindTag, tags.KubePod},
			"Sum of container runtime memory usage for Datadog Cluster Agent pods",
		),
		memoryLimits: tm.NewGauge(
			subsystem,
			MemoryLimit,
			[]string{kindTag, tags.KubePod},
			"Sum of container runtime memory limits for Datadog Cluster Agent pods",
		),
	}
}

// ResetRuntimeMetrics clears runtime-sourced memory aggregates.
func (t *Recorder) ResetRuntimeMetrics() {
	t.resetRuntimeMetrics()
}

// ResetKubeletMetrics clears kubelet-sourced state aggregates.
func (t *Recorder) ResetKubeletMetrics() {
	t.resetKubeletMetrics()
}

// RecordMetric adds a metric to the COAT aggregate when it belongs to
// a Datadog Cluster Agent or Cluster Check Runner pod.
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
	for _, kind := range []string{clusterAgentComponent, clusterChecksAgentComponentOperator} {
		match := map[string]string{kindTag: kind}
		t.memoryUsage.DeletePartialMatch(match)
		t.memoryLimits.DeletePartialMatch(match)
	}
}

func (t *Recorder) resetKubeletMetrics() {
	for _, kind := range []string{clusterAgentComponent, clusterChecksAgentComponentOperator} {
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
	case clusterAgentComponent:
		return component, true
	case clusterChecksAgentComponentHelm, clusterChecksAgentComponentOperator:
		// consolidate component name difference between helm and operator
		return clusterChecksAgentComponentOperator, true
	}

	return "", false
}
