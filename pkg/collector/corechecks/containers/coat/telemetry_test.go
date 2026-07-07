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
)

func TestAgentPodCOATComponent(t *testing.T) {
	tests := []struct {
		name              string
		tags              []string
		expectedComponent string
		expectedOK        bool
	}{
		{
			name:              "cluster agent",
			tags:              []string{"kube_namespace:datadog", "kube_app_component:cluster-agent"},
			expectedComponent: clusterAgentComponent,
			expectedOK:        true,
		},
		{
			name:              "cluster checks agent",
			tags:              []string{"kube_app_component:clusterchecks-agent", "pod_name:runner"},
			expectedComponent: clusterChecksAgentComponent,
			expectedOK:        true,
		},
		{
			name:       "other component",
			tags:       []string{"kube_app_component:agent"},
			expectedOK: false,
		},
		{
			name:       "missing component",
			tags:       []string{"pod_name:cluster-agent"},
			expectedOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			component, ok := agentPodCOATComponent(tt.tags)
			assert.Equal(t, tt.expectedOK, ok)
			assert.Equal(t, tt.expectedComponent, component)
		})
	}
}

func TestAgentPodCOATTelemetryAggregatesSelectedComponents(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodCOATTelemetry(tel)
	coat.resetKubeletMetrics()
	coat.resetRuntimeMetrics()

	coat.record(AgentMemoryUsage, 10, []string{"kube_app_component:cluster-agent", "pod_name:first"})
	coat.record(AgentMemoryUsage, 5, []string{"kube_app_component:cluster-agent", "pod_name:second"})
	coat.record(AgentMemoryUsage, 99, []string{"kube_app_component:agent"})
	coat.record(AgentMemoryLimit, 20, []string{"kube_app_component:clusterchecks-agent"})
	coat.record(AgentContainerRestarts, 2, []string{"kube_app_component:clusterchecks-agent"})
	coat.record(AgentContainerTerminated, 1, []string{"kube_app_component:cluster-agent", "reason:oomkilled"})
	coat.record(AgentContainerTerminated, 99, []string{"kube_app_component:cluster-agent"})

	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 15)
	assertGaugeValue(t, tel, AgentMemoryUsage, clusterChecksAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterChecksAgentComponent, 20)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterChecksAgentComponent, 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "oomkilled", 1)
}

func TestAgentPodCOATTelemetryResetClearsStaleValues(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodCOATTelemetry(tel)

	coat.record(AgentMemoryUsage, 10, []string{"kube_app_component:cluster-agent"})
	coat.record(AgentMemoryLimit, 20, []string{"kube_app_component:clusterchecks-agent"})
	coat.record(AgentContainerRestarts, 2, []string{"kube_app_component:clusterchecks-agent"})
	coat.record(AgentContainerTerminated, 1, []string{"kube_app_component:cluster-agent", "reason:error"})
	coat.resetKubeletMetrics()
	coat.resetRuntimeMetrics()

	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterChecksAgentComponent, 0)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterChecksAgentComponent, 0)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "error", 0)
}

func TestAgentPodCOATTelemetrySplitResets(t *testing.T) {
	tel := telemetrymock.New(t)
	coat := newAgentPodCOATTelemetry(tel)

	coat.record(AgentMemoryUsage, 10, []string{"kube_app_component:cluster-agent"})
	coat.record(AgentMemoryLimit, 20, []string{"kube_app_component:cluster-agent"})
	coat.record(AgentContainerRestarts, 2, []string{"kube_app_component:cluster-agent"})
	coat.record(AgentContainerTerminated, 1, []string{"kube_app_component:cluster-agent", "reason:containercannotrun"})

	coat.resetRuntimeMetrics()

	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterAgentComponent, 0)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterAgentComponent, 2)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "containercannotrun", 1)

	coat.record(AgentMemoryUsage, 10, []string{"kube_app_component:cluster-agent"})
	coat.record(AgentMemoryLimit, 20, []string{"kube_app_component:cluster-agent"})
	coat.resetKubeletMetrics()

	assertGaugeValue(t, tel, AgentMemoryUsage, clusterAgentComponent, 10)
	assertGaugeValue(t, tel, AgentMemoryLimit, clusterAgentComponent, 20)
	assertGaugeValue(t, tel, AgentContainerRestarts, clusterAgentComponent, 0)
	assertTerminatedGaugeValue(t, tel, clusterAgentComponent, "containercannotrun", 0)
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
