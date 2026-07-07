// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package coat

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestAgentPodCOATComponent(t *testing.T) {
	tests := []struct {
		name              string
		pod               *workloadmeta.KubernetesPod
		expectedComponent string
		expectedOK        bool
	}{
		{
			name:              "cluster agent",
			pod:               newTestPod(clusterAgentComponent),
			expectedComponent: clusterAgentComponent,
			expectedOK:        true,
		},
		{
			name:              "cluster checks agent",
			pod:               newTestPod(clusterChecksAgentComponent),
			expectedComponent: clusterChecksAgentComponent,
			expectedOK:        true,
		},
		{
			name:       "other component",
			pod:        newTestPod("agent"),
			expectedOK: false,
		},
		{
			name:       "missing component",
			pod:        newTestPod(""),
			expectedOK: false,
		},
		{
			name:       "nil pod",
			pod:        nil,
			expectedOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component, ok := agentPodComponent(tt.pod)
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedComponent, component)
		})
	}
}

func TestRecordAgentMetricUsesPodLabel(t *testing.T) {
	tel := telemetrymock.New(t)
	t.Cleanup(setAgentPodTelemetryForTest(tel))
	ResetAgentRuntimeMetrics()
	ResetAgentKubeletMetrics()

	RecordAgentMetric(AgentCPUUsage, ptr(100), newTestPod(clusterAgentComponent), "")
	RecordAgentMetric(AgentCPUUsage, ptr(99), newTestPod("agent"), "")
	RecordAgentMetric(AgentContainerTerminated, ptr(1), newTestPod(clusterAgentComponent), "oomkilled")
	RecordAgentMetric(AgentContainerTerminated, ptr(99), newTestPod(clusterAgentComponent), "")

	assertGaugeValue(t, tel, AgentCPUUsage, clusterAgentComponent, 100)
	assertGaugeValue(t, tel, AgentCPUUsage, clusterChecksAgentComponent, 0)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "oomkilled", 1)
}

func TestAgentPodCOATTelemetryAggregatesSelectedComponents(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodTelemetry(tel)
	coat.resetKubeletMetrics()
	coat.resetRuntimeMetrics()

	coat.record(AgentCPUUsage, 100, clusterAgentComponent, "")
	coat.record(AgentCPUUsage, 50, clusterAgentComponent, "")
	coat.record(AgentMemoryUsage, 10, clusterAgentComponent, "")
	coat.record(AgentMemoryUsage, 5, clusterAgentComponent, "")
	coat.record(AgentMemoryLimit, 20, clusterChecksAgentComponent, "")
	coat.record(AgentContainerRestarts, 2, clusterChecksAgentComponent, "")
	coat.record(AgentContainerTerminated, 1, clusterAgentComponent, "oomkilled")
	coat.record(AgentContainerTerminated, 99, clusterAgentComponent, "")

	assertGaugeValue(t, tel, AgentCPUUsage, clusterAgentComponent, 150)
	assertGaugeValue(t, tel, AgentCPUUsage, clusterChecksAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 15)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterChecksAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterChecksAgentComponent, 20)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterChecksAgentComponent, 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "oomkilled", 1)
}

func TestAgentPodCOATTelemetryResetClearsStaleValues(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodTelemetry(tel)

	coat.record(AgentCPUUsage, 100, clusterAgentComponent, "")
	coat.record(AgentMemoryUsage, 10, clusterAgentComponent, "")
	coat.record(AgentMemoryLimit, 20, clusterChecksAgentComponent, "")
	coat.record(AgentContainerRestarts, 2, clusterChecksAgentComponent, "")
	coat.record(AgentContainerTerminated, 1, clusterAgentComponent, "error")
	coat.resetKubeletMetrics()
	coat.resetRuntimeMetrics()

	assertGaugeValue(t, tel, AgentCPUUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterChecksAgentComponent, 0)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterChecksAgentComponent, 0)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "error", 0)
}

func TestAgentPodCOATTelemetrySplitResets(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodTelemetry(tel)

	coat.record(AgentCPUUsage, 100, clusterAgentComponent, "")
	coat.record(AgentMemoryUsage, 10, clusterAgentComponent, "")
	coat.record(AgentMemoryLimit, 20, clusterAgentComponent, "")
	coat.record(AgentContainerRestarts, 2, clusterAgentComponent, "")
	coat.record(AgentContainerTerminated, 1, clusterAgentComponent, "containercannotrun")

	coat.resetRuntimeMetrics()

	assertGaugeValue(t, tel, AgentCPUUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterAgentComponent, 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "containercannotrun", 1)

	coat.record(AgentCPUUsage, 100, clusterAgentComponent, "")
	coat.record(AgentMemoryUsage, 10, clusterAgentComponent, "")
	coat.record(AgentMemoryLimit, 20, clusterAgentComponent, "")
	coat.resetKubeletMetrics()

	assertGaugeValue(t, tel, AgentCPUUsage, clusterAgentComponent, 100)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 10)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterAgentComponent, 20)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterAgentComponent, 0)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "containercannotrun", 0)
}

func newTestPod(component string) *workloadmeta.KubernetesPod {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod"},
		EntityMeta: workloadmeta.EntityMeta{
			Labels: map[string]string{},
		},
	}
	if component != "" {
		pod.Labels[kubernetes.KubeAppComponentLabelKey] = component
	}
	return pod
}

func ptr(v float64) *float64 {
	return &v
}

func assertGaugeValue(t *testing.T, tel telemetry.Mock, metricName string, component string, expected float64) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(agentSubsystem, metricName)
	if !assert.NoError(t, err) {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[tags.KubeAppComponent] == component {
			assert.Equal(t, expected, metric.Value())
			return
		}
	}

	assert.Failf(t, "missing metric", "metric %s for %s not found", metricName, component)
}

func assertTerminatedGaugeValue(t *testing.T, tel telemetry.Mock, component string, reason string, expected float64) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(agentSubsystem, AgentContainerTerminated)
	if !assert.NoError(t, err) {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[tags.KubeAppComponent] == component && metric.Tags()["reason"] == reason {
			assert.Equal(t, expected, metric.Value())
			return
		}
	}

	assert.Failf(t, "missing metric", "terminated metric for %s/%s not found", component, reason)
}

// setAgentPodTelemetryForTest replaces the package telemetry instance and
// returns a cleanup function that restores the previous instance.
func setAgentPodTelemetryForTest(tm telemetry.Component) func() {
	previous := agentTelemetry
	agentTelemetry = newAgentPodTelemetry(tm)
	return func() {
		agentTelemetry = previous
	}
}
