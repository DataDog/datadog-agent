// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package coat is used to collect container/kubernetes metrics of agents running in a kubernetes cluster.
package coat

import (
	"sync"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	agentSubsystem = "kubernetes_agent"
	// AgentContainerRestarts is the COAT metric name for Kubernetes container restarts.
	AgentContainerRestarts = "containers_restarts"
	// AgentContainerTerminated is the COAT metric name for Kubernetes container terminated states.
	AgentContainerTerminated = "containers_terminated"
	// AgentMemoryUsage is the COAT metric name for container runtime memory usage.
	AgentMemoryUsage = "memory_usage"
	// AgentMemoryLimit is the COAT metric name for container runtime memory limits.
	AgentMemoryLimit = "memory_limit"

	clusterAgentComponent               = "cluster-agent"
	clusterChecksAgentComponentHelm     = "clusterchecks-agent"
	clusterChecksAgentComponentOperator = "cluster-checks-runner"
)

var (
	defaultAgentPodTelemetry     *AgentPodTelemetry
	defaultAgentPodTelemetryOnce sync.Once
)

// AgentPodTelemetry records COAT metrics for Datadog Agent pods.
type AgentPodTelemetry struct {
	containersRestarts   telemetry.Gauge
	containersTerminated telemetry.Gauge
	memoryUsage          telemetry.Gauge
	memoryLimits         telemetry.Gauge
}

// NewAgentPodTelemetry returns the shared COAT recorder for Datadog Agent pods.
func NewAgentPodTelemetry(tm telemetry.Component) *AgentPodTelemetry {
	defaultAgentPodTelemetryOnce.Do(func() {
		defaultAgentPodTelemetry = newAgentPodTelemetry(tm)
	})
	return defaultAgentPodTelemetry
}

func newAgentPodTelemetry(tm telemetry.Component) *AgentPodTelemetry {
	return &AgentPodTelemetry{
		containersRestarts: tm.NewGauge(
			agentSubsystem,
			AgentContainerRestarts,
			[]string{tags.KubeAppComponent, tags.KubePod},
			"Sum of kubernetes.containers.restarts for Datadog Cluster Agent pods",
		),
		containersTerminated: tm.NewGauge(
			agentSubsystem,
			AgentContainerTerminated,
			[]string{tags.KubeAppComponent, tags.KubePod, "reason"},
			"Sum of kubernetes.containers.*.terminated for Datadog Cluster Agent pods",
		),
		memoryUsage: tm.NewGauge(
			agentSubsystem,
			AgentMemoryUsage,
			[]string{tags.KubeAppComponent, tags.KubePod},
			"Sum of container runtime memory usage for Datadog Cluster Agent pods",
		),
		memoryLimits: tm.NewGauge(
			agentSubsystem,
			AgentMemoryLimit,
			[]string{tags.KubeAppComponent, tags.KubePod},
			"Sum of container runtime memory limits for Datadog Cluster Agent pods",
		),
	}
}

// ResetRuntimeMetrics clears runtime-sourced memory aggregates.
func (t *AgentPodTelemetry) ResetRuntimeMetrics() {
	t.resetRuntimeMetrics()
}

// ResetKubeletMetrics clears kubelet-sourced state aggregates.
func (t *AgentPodTelemetry) ResetKubeletMetrics() {
	t.resetKubeletMetrics()
}

// RecordMetric adds a metric to the COAT aggregate when it belongs to
// a Datadog Cluster Agent or Cluster Check Runner pod.
func (t *AgentPodTelemetry) RecordMetric(metricName string, value *float64, pod *workloadmeta.KubernetesPod, reason string) {
	if value == nil || pod == nil {
		return
	}

	component, ok := agentPodComponent(pod)
	if !ok {
		return
	}

	if pod.Name == "" {
		return
	}
	t.record(metricName, *value, component, pod.Name, reason)
}

func (t *AgentPodTelemetry) resetRuntimeMetrics() {
	for _, component := range []string{clusterAgentComponent, clusterChecksAgentComponentOperator} {
		match := map[string]string{tags.KubeAppComponent: component}
		t.memoryUsage.DeletePartialMatch(match)
		t.memoryLimits.DeletePartialMatch(match)
	}
}

func (t *AgentPodTelemetry) resetKubeletMetrics() {
	for _, component := range []string{clusterAgentComponent, clusterChecksAgentComponentOperator} {
		match := map[string]string{tags.KubeAppComponent: component}
		t.containersRestarts.DeletePartialMatch(match)
		t.containersTerminated.DeletePartialMatch(match)
	}
}

func (t *AgentPodTelemetry) record(metricName string, value float64, component string, podName string, reason string) {
	switch metricName {
	case AgentContainerRestarts:
		t.containersRestarts.Add(value, component, podName)
	case AgentContainerTerminated:
		if reason == "" {
			return
		}
		t.containersTerminated.Add(value, component, podName, reason)
	case AgentMemoryUsage:
		t.memoryUsage.Add(value, component, podName)
	case AgentMemoryLimit:
		t.memoryLimits.Add(value, component, podName)
	}
}

func agentPodComponent(pod *workloadmeta.KubernetesPod) (string, bool) {
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
