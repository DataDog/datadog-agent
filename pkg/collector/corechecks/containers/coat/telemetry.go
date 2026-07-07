// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package coat

import (
	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetryimpl "github.com/DataDog/datadog-agent/comp/core/telemetry/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

const (
	agentSubsystem = "kubernetes_agent"
	// AgentContainerRestarts is the COAT metric name for Kubernetes container restarts.
	AgentContainerRestarts = "containers_restarts"
	// AgentContainerTerminated is the COAT metric name for Kubernetes container terminated states.
	AgentContainerTerminated = "containers_terminated"
	// AgentCPUUsage is the COAT metric name for container runtime CPU usage.
	AgentCPUUsage = "cpu_usage"
	// AgentMemoryUsage is the COAT metric name for container runtime memory usage.
	AgentMemoryUsage = "memory_usage"
	// AgentMemoryLimit is the COAT metric name for container runtime memory limits.
	AgentMemoryLimit = "memory_limits"

	clusterAgentComponent       = "cluster-agent"
	clusterChecksAgentComponent = "clusterchecks-agent"
)

var agentContainerTerminatedReasons = []string{"oomkilled", "containercannotrun", "error"}

var agentTelemetry = newAgentPodTelemetry(telemetryimpl.GetCompatComponent())

type agentPodTelemetry struct {
	containersRestarts   telemetry.Gauge
	containersTerminated telemetry.Gauge
	cpuUsage             telemetry.Gauge
	memoryUsage          telemetry.Gauge
	memoryLimits         telemetry.Gauge
}

func newAgentPodTelemetry(tm telemetry.Component) *agentPodTelemetry {
	return &agentPodTelemetry{
		containersRestarts: tm.NewGauge(
			agentSubsystem,
			AgentContainerRestarts,
			[]string{tags.KubeAppComponent},
			"Sum of kubernetes.containers.restarts for Datadog Cluster Agent pods",
		),
		containersTerminated: tm.NewGauge(
			agentSubsystem,
			AgentContainerTerminated,
			[]string{tags.KubeAppComponent, "reason"},
			"Sum of kubernetes.containers.*.terminated for Datadog Cluster Agent pods",
		),
		cpuUsage: tm.NewGauge(
			agentSubsystem,
			AgentCPUUsage,
			[]string{tags.KubeAppComponent},
			"Sum of container runtime CPU usage for Datadog Cluster Agent pods",
		),
		memoryUsage: tm.NewGauge(
			agentSubsystem,
			AgentMemoryUsage,
			[]string{tags.KubeAppComponent},
			"Sum of container runtime memory usage for Datadog Cluster Agent pods",
		),
		memoryLimits: tm.NewGauge(
			agentSubsystem,
			AgentMemoryLimit,
			[]string{tags.KubeAppComponent},
			"Sum of container runtime memory limits for Datadog Cluster Agent pods",
		),
	}
}

// ResetAgentRuntimeMetrics clears runtime-sourced memory aggregates.
func ResetAgentRuntimeMetrics() {
	agentTelemetry.resetRuntimeMetrics()
}

// ResetAgentKubeletMetrics clears kubelet-sourced state aggregates.
func ResetAgentKubeletMetrics() {
	agentTelemetry.resetKubeletMetrics()
}

// RecordAgentMetric adds a metric to the COAT aggregate when it belongs to
// a Datadog Cluster Agent or Cluster Check Runner pod.
func RecordAgentMetric(metricName string, value *float64, pod *workloadmeta.KubernetesPod, reason string) {
	if value == nil {
		return
	}
	component, ok := agentPodComponent(pod)
	if !ok {
		return
	}
	agentTelemetry.record(metricName, *value, component, reason)
}

func (t *agentPodTelemetry) resetRuntimeMetrics() {
	for _, component := range []string{clusterAgentComponent, clusterChecksAgentComponent} {
		t.cpuUsage.Set(0, component)
		t.memoryUsage.Set(0, component)
		t.memoryLimits.Set(0, component)
	}
}

func (t *agentPodTelemetry) resetKubeletMetrics() {
	for _, component := range []string{clusterAgentComponent, clusterChecksAgentComponent} {
		t.containersRestarts.Set(0, component)
		for _, reason := range agentContainerTerminatedReasons {
			t.containersTerminated.Set(0, component, reason)
		}
	}
}

func (t *agentPodTelemetry) record(metricName string, value float64, component string, reason string) {
	switch metricName {
	case AgentContainerRestarts:
		t.containersRestarts.Add(value, component)
	case AgentContainerTerminated:
		if reason == "" {
			return
		}
		t.containersTerminated.Add(value, component, reason)
	case AgentCPUUsage:
		t.cpuUsage.Add(value, component)
	case AgentMemoryUsage:
		t.memoryUsage.Add(value, component)
	case AgentMemoryLimit:
		t.memoryLimits.Add(value, component)
	}
}

func agentPodComponent(pod *workloadmeta.KubernetesPod) (string, bool) {
	if pod == nil {
		return "", false
	}
	switch component := pod.Labels[kubernetes.KubeAppComponentLabelKey]; component {
	case clusterAgentComponent, clusterChecksAgentComponent:
		return component, true
	}

	return "", false
}
