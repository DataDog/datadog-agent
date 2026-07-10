// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentperformance

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

func TestRecorderComponent(t *testing.T) {
	tests := []struct {
		name         string
		pod          *workloadmeta.KubernetesPod
		expectedKind string
		expectedOK   bool
	}{
		{
			name:         "cluster agent",
			pod:          newTestPod(clusterAgentComponent, "metadata-pod"),
			expectedKind: clusterAgentComponent,
			expectedOK:   true,
		},
		{
			name:         "cluster checks agent helm",
			pod:          newTestPod(clusterChecksAgentComponentHelm, "metadata-pod"),
			expectedKind: clusterChecksAgentComponentOperator,
			expectedOK:   true,
		},
		{
			name:         "cluster checks agent operator",
			pod:          newTestPod(clusterChecksAgentComponentOperator, "metadata-pod"),
			expectedKind: clusterChecksAgentComponentOperator,
			expectedOK:   true,
		},
		{
			name:       "other component",
			pod:        newTestPod("agent", "metadata-pod"),
			expectedOK: false,
		},
		{
			name:       "missing component",
			pod:        newTestPod("", "metadata-pod"),
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
			kind, ok := agentPodKind(tt.pod)
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedKind, kind)
		})
	}
}

func TestRecordAgentMetricUsesPodMetadata(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)
	agentPerformance.ResetRuntimeMetrics()
	agentPerformance.ResetKubeletMetrics()

	agentPerformance.RecordMetric(MemoryUsage, ptr(100), newTestPod(clusterAgentComponent, "metadata-pod"), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(50), newTestPod(clusterChecksAgentComponentHelm, "clusterchecks-agent-helm-pod"), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(25), newTestPod(clusterChecksAgentComponentOperator, "clusterchecks-agent-operator-pod"), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(99), newTestPod("agent", "other-pod"), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(98), newTestPod(clusterAgentComponent, ""), "")
	agentPerformance.RecordMetric(ContainerTerminated, ptr(1), newTestPod(clusterAgentComponent, "metadata-pod"), "oomkilled")
	agentPerformance.RecordMetric(ContainerTerminated, ptr(99), newTestPod(clusterAgentComponent, "metadata-pod"), "")

	assertGaugeValue(t, tel, MemoryUsage, clusterAgentComponent, "metadata-pod", 100)
	assertGaugeValue(t, tel, MemoryUsage, clusterChecksAgentComponentOperator, "clusterchecks-agent-helm-pod", 50)
	assertGaugeValue(t, tel, MemoryUsage, clusterChecksAgentComponentOperator, "clusterchecks-agent-operator-pod", 25)
	assertGaugeMissing(t, tel, MemoryUsage, "agent", "other-pod")
	assertGaugeMissing(t, tel, MemoryUsage, clusterAgentComponent, "")
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "metadata-pod", "oomkilled", 1)
}

func TestRecorderTelemetryAggregatesSelectedComponents(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)
	agentPerformance.resetKubeletMetrics()
	agentPerformance.resetRuntimeMetrics()

	agentPerformance.record(MemoryUsage, 10, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryUsage, 5, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryLimit, 20, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", "")
	agentPerformance.record(ContainerRestarts, 2, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", "")
	agentPerformance.record(ContainerTerminated, 1, clusterAgentComponent, "cluster-agent-pod", "oomkilled")
	agentPerformance.record(ContainerTerminated, 99, clusterAgentComponent, "cluster-agent-pod", "")

	assertGaugeValue(t, tel, MemoryUsage, clusterAgentComponent, "cluster-agent-pod", 15)
	assertGaugeValue(t, tel, MemoryLimit, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", 20)
	assertGaugeValue(t, tel, ContainerRestarts, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "cluster-agent-pod", "oomkilled", 1)
}

func TestRecorderTelemetryResetClearsStaleValues(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)

	agentPerformance.record(MemoryUsage, 10, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryLimit, 20, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", "")
	agentPerformance.record(ContainerRestarts, 2, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod", "")
	agentPerformance.record(ContainerTerminated, 1, clusterAgentComponent, "cluster-agent-pod", "error")
	agentPerformance.resetKubeletMetrics()
	agentPerformance.resetRuntimeMetrics()

	assertGaugeMissing(t, tel, MemoryUsage, clusterAgentComponent, "cluster-agent-pod")
	assertGaugeMissing(t, tel, MemoryLimit, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod")
	assertGaugeMissing(t, tel, ContainerRestarts, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod")
	assertTerminatedGaugeMissing(t, tel, clusterAgentComponent, "cluster-agent-pod", "error")
}

func TestRecorderTelemetrySplitResets(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)

	agentPerformance.record(MemoryUsage, 10, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryLimit, 20, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(ContainerRestarts, 2, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(ContainerTerminated, 1, clusterAgentComponent, "cluster-agent-pod", "containercannotrun")

	agentPerformance.resetRuntimeMetrics()

	assertGaugeMissing(t, tel, MemoryUsage, clusterAgentComponent, "cluster-agent-pod")
	assertGaugeMissing(t, tel, MemoryLimit, clusterAgentComponent, "cluster-agent-pod")
	assertGaugeValue(t, tel, ContainerRestarts, clusterAgentComponent, "cluster-agent-pod", 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "cluster-agent-pod", "containercannotrun", 1)

	agentPerformance.record(MemoryUsage, 10, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryLimit, 20, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.resetKubeletMetrics()

	assertGaugeValue(t, tel, MemoryUsage, clusterAgentComponent, "cluster-agent-pod", 10)
	assertGaugeValue(t, tel, MemoryLimit, clusterAgentComponent, "cluster-agent-pod", 20)
	assertGaugeMissing(t, tel, ContainerRestarts, clusterAgentComponent, "cluster-agent-pod")
	assertTerminatedGaugeMissing(t, tel, clusterAgentComponent, "cluster-agent-pod", "containercannotrun")
}

func newTestPod(component string, podName string) *workloadmeta.KubernetesPod {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod"},
		EntityMeta: workloadmeta.EntityMeta{
			Name:   podName,
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

func assertGaugeValue(t *testing.T, tel telemetry.Mock, metricName string, kind string, podName string, expected float64) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(subsystem, metricName)
	if !assert.NoError(t, err) {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[kindTag] == kind && metric.Tags()[tags.KubePod] == podName {
			assert.Equal(t, expected, metric.Value())
			return
		}
	}

	assert.Failf(t, "missing metric", "metric %s for %s/%s not found", metricName, kind, podName)
}

func assertGaugeMissing(t *testing.T, tel telemetry.Mock, metricName string, kind string, podName string) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(subsystem, metricName)
	if err != nil {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[kindTag] == kind && metric.Tags()[tags.KubePod] == podName {
			assert.Failf(t, "unexpected metric", "metric %s for %s/%s found", metricName, kind, podName)
			return
		}
	}
}

func assertTerminatedGaugeMissing(t *testing.T, tel telemetry.Mock, kind string, podName string, reason string) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(subsystem, ContainerTerminated)
	if err != nil {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[kindTag] == kind && metric.Tags()[tags.KubePod] == podName && metric.Tags()["reason"] == reason {
			assert.Failf(t, "unexpected metric", "terminated metric for %s/%s/%s found", kind, podName, reason)
			return
		}
	}
}

func assertTerminatedGaugeValue(t *testing.T, tel telemetry.Mock, kind string, podName string, reason string, expected float64) {
	t.Helper()

	metrics, err := tel.GetGaugeMetric(subsystem, ContainerTerminated)
	if !assert.NoError(t, err) {
		return
	}

	for _, metric := range metrics {
		if metric.Tags()[kindTag] == kind && metric.Tags()[tags.KubePod] == podName && metric.Tags()["reason"] == reason {
			assert.Equal(t, expected, metric.Value())
			return
		}
	}

	assert.Failf(t, "missing metric", "terminated metric for %s/%s/%s not found", kind, podName, reason)
}
