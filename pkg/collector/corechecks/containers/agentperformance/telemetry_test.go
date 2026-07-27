// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agentperformance

import (
	"errors"
	"math"
	"testing"
	"time"

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
			name:         "node agent",
			pod:          newNodeAgentTestPod("metadata-pod"),
			expectedKind: nodeAgentComponent,
			expectedOK:   true,
		},
		{
			name:       "node agent missing Datadog identity",
			pod:        newTestPod(nodeAgentComponent, "metadata-pod"),
			expectedOK: false,
		},
		{
			name:       "unrelated component",
			pod:        newTestPod("unrelated-component", "metadata-pod"),
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
	agentPerformance.RecordMetric(MemoryUsage, ptr(99), newTestPod("unrelated-component", "other-pod"), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(98), newTestPod(clusterAgentComponent, ""), "")
	agentPerformance.RecordMetric(ContainerTerminated, ptr(1), newTestPod(clusterAgentComponent, "metadata-pod"), "oomkilled")
	agentPerformance.RecordMetric(ContainerTerminated, ptr(99), newTestPod(clusterAgentComponent, "metadata-pod"), "")

	assertGaugeValue(t, tel, MemoryUsage, clusterAgentComponent, "metadata-pod", 100)
	assertGaugeValue(t, tel, MemoryUsage, clusterChecksAgentComponentOperator, "clusterchecks-agent-helm-pod", 50)
	assertGaugeValue(t, tel, MemoryUsage, clusterChecksAgentComponentOperator, "clusterchecks-agent-operator-pod", 25)
	assertGaugeMissing(t, tel, MemoryUsage, "unrelated-component", "other-pod")
	assertGaugeMissing(t, tel, MemoryUsage, clusterAgentComponent, "")
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "metadata-pod", "oomkilled", 1)
}

func TestRecorderTelemetryAggregatesNodeAgentPodMetrics(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)
	agentPerformance.ResetRuntimeMetrics()

	nodeAgentPod := newNodeAgentTestPod("node-agent-pod")
	agentPerformance.RecordMetric(MemoryUsage, ptr(10), nodeAgentPod, "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(5), nodeAgentPod, "")

	assertGaugeValue(t, tel, MemoryUsage, "agent", "node-agent-pod", 15)
}

func TestRecorderTelemetryKeepsNodeAndClusterAgentsSeparateByKind(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)
	agentPerformance.ResetRuntimeMetrics()

	const podName = "agent-pod"
	agentPerformance.RecordMetric(MemoryUsage, ptr(10), newTestPod(clusterAgentComponent, podName), "")
	agentPerformance.RecordMetric(MemoryUsage, ptr(5), newNodeAgentTestPod(podName), "")

	assertGaugeValue(t, tel, MemoryUsage, clusterAgentComponent, podName, 10)
	assertGaugeValue(t, tel, MemoryUsage, "agent", podName, 5)
}

func TestRecorderRuntimeMetricsCallbackSerializesCPUSnapshots(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	firstCallbackEntered := make(chan struct{})
	releaseFirstCallback := make(chan struct{})
	firstResult := make(chan error, 1)
	expectedErr := errors.New("first callback failed")

	go func() {
		firstResult <- recorder.WithRuntimeMetrics(func() error {
			recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), time.Unix(100, 0), newNodeAgentTestPod("node-agent-pod"))
			close(firstCallbackEntered)
			<-releaseFirstCallback
			return expectedErr
		})
	}()

	<-firstCallbackEntered
	assert.False(t, recorder.runtimeSnapshotMu.TryLock())

	close(releaseFirstCallback)
	assert.ErrorIs(t, <-firstResult, expectedErr)
	assert.Contains(t, recorder.previousCPUSamples, "node-agent-container")
	if assert.True(t, recorder.runtimeSnapshotMu.TryLock()) {
		recorder.runtimeSnapshotMu.Unlock()
	}
}

func TestRecorderCPUUsageFirstSampleDoesNotEmitGauge(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), time.Unix(100, 0), newNodeAgentTestPod("node-agent-pod"))
	recorder.CompleteRuntimeMetrics()

	assertGaugeMissing(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod")
}

func TestRecorderCPUUsageInvalidTotalsDoNotEmitOrReplaceBaseline(t *testing.T) {
	tests := []struct {
		name         string
		invalidTotal float64
	}{
		{name: "negative", invalidTotal: -float64(time.Second)},
		{name: "NaN", invalidTotal: math.NaN()},
		{name: "positive infinity", invalidTotal: math.Inf(1)},
		{name: "negative infinity", invalidTotal: math.Inf(-1)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tel := telemetrymock.New(t)
			recorder := newRecorder(tel)
			pod := newNodeAgentTestPod("node-agent-pod")
			collectionStart := time.Unix(0, 0)

			recorder.ResetRuntimeMetrics()
			recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), collectionStart, pod)
			recorder.CompleteRuntimeMetrics()

			recorder.ResetRuntimeMetrics()
			recorder.RecordCPUUsage("node-agent-container", ptr(tt.invalidTotal), collectionStart.Add(10*time.Second), pod)
			assert.NotContains(t, recorder.currentCPUSamples, "node-agent-container")
			recorder.CompleteRuntimeMetrics()
			assertGaugeMissing(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod")

			recorder.ResetRuntimeMetrics()
			recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(20*time.Second), pod)
			recorder.CompleteRuntimeMetrics()
			assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod", 0.125)
		})
	}
}

func TestRecorderCPUUsageAggregatesNodeAgentContainers(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	nodeAgentPod := newNodeAgentTestPod("node-agent-pod")
	collectionStart := time.Unix(100, 0)

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container-1", ptr(float64(time.Second)), collectionStart, nodeAgentPod)
	recorder.RecordCPUUsage("node-agent-container-2", ptr(float64(2*time.Second)), collectionStart, nodeAgentPod)
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container-1", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(10*time.Second), nodeAgentPod)
	recorder.RecordCPUUsage("node-agent-container-2", ptr(float64(4*time.Second)), collectionStart.Add(10*time.Second), nodeAgentPod)
	recorder.CompleteRuntimeMetrics()

	assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod", 0.45)
}

func TestRecorderCPUUsageKeepsNodeAndClusterAgentsSeparateByKind(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	collectionStart := time.Unix(100, 0)
	nodeAgentPod := newNodeAgentTestPod("agent-pod")
	clusterAgentPod := newTestPod(clusterAgentComponent, "agent-pod")

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), collectionStart, nodeAgentPod)
	recorder.RecordCPUUsage("cluster-agent-container", ptr(float64(2*time.Second)), collectionStart, clusterAgentPod)
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(10*time.Second), nodeAgentPod)
	recorder.RecordCPUUsage("cluster-agent-container", ptr(float64(3*time.Second)), collectionStart.Add(10*time.Second), clusterAgentPod)
	recorder.CompleteRuntimeMetrics()

	assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "agent-pod", 0.25)
	assertGaugeValue(t, tel, CPUUsage, clusterAgentComponent, "agent-pod", 0.1)
}

func TestRecorderCPUUsageDecreasedCounterReestablishesBaseline(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	pod := newNodeAgentTestPod("node-agent-pod")
	collectionStart := time.Unix(100, 0)

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second)), collectionStart, pod)
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), collectionStart.Add(10*time.Second), pod)
	recorder.CompleteRuntimeMetrics()

	assertGaugeMissing(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod")

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(20*time.Second), pod)
	recorder.CompleteRuntimeMetrics()

	assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod", 0.25)
}

func TestRecorderCPUUsageRetainsBaselineForListedContainerWithoutCPUStats(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	pod := newNodeAgentTestPod("node-agent-pod")
	collectionStart := time.Unix(100, 0)

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), collectionStart, pod)
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.MarkCPUContainerPresent("node-agent-container")
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(20*time.Second), pod)
	recorder.CompleteRuntimeMetrics()

	assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod", 0.125)
}

func TestRecorderCPUUsageEmptyLaterSnapshotClearsOldContribution(t *testing.T) {
	tel := telemetrymock.New(t)
	recorder := newRecorder(tel)
	pod := newNodeAgentTestPod("node-agent-pod")
	collectionStart := time.Unix(100, 0)

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(time.Second)), collectionStart, pod)
	recorder.CompleteRuntimeMetrics()

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(3*time.Second+500*time.Millisecond)), collectionStart.Add(10*time.Second), pod)
	recorder.CompleteRuntimeMetrics()
	assertGaugeValue(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod", 0.25)

	recorder.ResetRuntimeMetrics()
	recorder.CompleteRuntimeMetrics()
	assertGaugeMissing(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod")

	recorder.ResetRuntimeMetrics()
	recorder.RecordCPUUsage("node-agent-container", ptr(float64(6*time.Second)), collectionStart.Add(30*time.Second), pod)
	recorder.CompleteRuntimeMetrics()
	assertGaugeMissing(t, tel, CPUUsage, nodeAgentComponent, "node-agent-pod")
}

func TestRecorderTelemetryAggregatesSelectedComponents(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)
	agentPerformance.resetKubeletMetrics()
	agentPerformance.ResetRuntimeMetrics()

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
	nodeAgentPod := newNodeAgentTestPod("node-agent-pod")
	agentPerformance.RecordMetric(MemoryUsage, ptr(30), nodeAgentPod, "")
	agentPerformance.RecordMetric(MemoryLimit, ptr(40), nodeAgentPod, "")
	agentPerformance.RecordMetric(ContainerRestarts, ptr(3), nodeAgentPod, "")
	agentPerformance.RecordMetric(ContainerTerminated, ptr(2), nodeAgentPod, "error")
	agentPerformance.resetKubeletMetrics()
	agentPerformance.ResetRuntimeMetrics()

	assertGaugeMissing(t, tel, MemoryUsage, clusterAgentComponent, "cluster-agent-pod")
	assertGaugeMissing(t, tel, MemoryLimit, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod")
	assertGaugeMissing(t, tel, ContainerRestarts, clusterChecksAgentComponentOperator, "clusterchecks-agent-pod")
	assertTerminatedGaugeMissing(t, tel, clusterAgentComponent, "cluster-agent-pod", "error")
	assertGaugeMissing(t, tel, MemoryUsage, "agent", "node-agent-pod")
	assertGaugeMissing(t, tel, MemoryLimit, "agent", "node-agent-pod")
	assertGaugeMissing(t, tel, ContainerRestarts, "agent", "node-agent-pod")
	assertTerminatedGaugeMissing(t, tel, "agent", "node-agent-pod", "error")
}

func TestRecorderTelemetrySplitResets(t *testing.T) {
	tel := telemetrymock.New(t)
	agentPerformance := newRecorder(tel)

	agentPerformance.record(MemoryUsage, 10, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(MemoryLimit, 20, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(ContainerRestarts, 2, clusterAgentComponent, "cluster-agent-pod", "")
	agentPerformance.record(ContainerTerminated, 1, clusterAgentComponent, "cluster-agent-pod", "containercannotrun")

	agentPerformance.ResetRuntimeMetrics()

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

func newNodeAgentTestPod(podName string) *workloadmeta.KubernetesPod {
	pod := newTestPod(nodeAgentComponent, podName)
	pod.Labels[datadogComponentLabelKey] = nodeAgentComponent
	return pod
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
