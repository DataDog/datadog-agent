// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
)

func newTestCollectorConsumer(buildInfo component.BuildInfo) *collectorConsumer {
	s := &serializerConsumer{ipath: ossCollector}
	return &collectorConsumer{
		serializerConsumer: s,
		seenHosts:          make(map[string]struct{}),
		seenTagSets:        make(map[string]tagSetEntry),
		buildInfo:          buildInfo,
		getPushTime:        func() uint64 { return uint64(2e9) },
	}
}

func TestExporterWorkloadMetrics(t *testing.T) {
	tests := []struct {
		name         string
		metricSuffix string
		tags         []string
		wantName     string
	}{
		{
			name:         "fargate",
			metricSuffix: "fargate",
			tags:         []string{"version:1.0", "command:otelcontribcol", "task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc"},
			wantName:     "otel.datadog_exporter.metrics.running.fargate",
		},
		{
			name:         "azurecontainerapps",
			metricSuffix: "azurecontainerapps",
			tags:         []string{"version:1.0", "command:otelcontribcol", "replica_name:replica-1"},
			wantName:     "otel.datadog_exporter.metrics.running.azurecontainerapps",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			serie := exporterWorkloadMetrics(tt.metricSuffix, uint64(2e9), tt.tags)

			assert.Equal(t, tt.wantName, serie.Name)
			assert.Equal(t, 1, len(serie.Points))
			assert.Equal(t, float64(2e9), serie.Points[0].Ts)
			assert.Equal(t, 1.0, serie.Points[0].Value)
			assert.Equal(t, "", serie.Host)
		})
	}
}

func TestAddRuntimeTelemetryMetric_NoTags(t *testing.T) {
	buildInfo := component.BuildInfo{Version: "1.0", Command: "otelcontribcol"}
	c := newTestCollectorConsumer(buildInfo)

	c.addRuntimeTelemetryMetric("", nil)

	assert.Empty(t, c.series)
}

func TestAddRuntimeTelemetryMetric_HostSource(t *testing.T) {
	buildInfo := component.BuildInfo{Version: "1.0", Command: "otelcontribcol"}
	c := newTestCollectorConsumer(buildInfo)
	c.ConsumeHost("my-hostname")

	c.addRuntimeTelemetryMetric("", nil)

	var names []string
	var hosts []string
	for _, s := range c.series {
		names = append(names, s.Name)
		hosts = append(hosts, s.Host)
	}
	// host path emits metrics.running with hostname, plus the baseline tagless metric
	assert.Contains(t, names, "otel.datadog_exporter.metrics.running")
	assert.Contains(t, hosts, "my-hostname")
}

func TestAddRuntimeTelemetryMetric_FargateTags(t *testing.T) {
	buildInfo := component.BuildInfo{Version: "1.0", Command: "otelcontribcol"}
	c := newTestCollectorConsumer(buildInfo)
	tag := "task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc"
	c.ConsumeTagSet("fargate", tag, []string{tag})

	c.addRuntimeTelemetryMetric("", nil)

	var names []string
	for _, s := range c.series {
		names = append(names, s.Name)
	}
	assert.ElementsMatch(t, []string{"otel.datadog_exporter.metrics.running.fargate"}, names)
}

func TestAddRuntimeTelemetryMetric_HostAndFargate(t *testing.T) {
	buildInfo := component.BuildInfo{Version: "1.0", Command: "otelcontribcol"}
	c := newTestCollectorConsumer(buildInfo)
	c.ConsumeHost("my-hostname")
	tag := "task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc"
	c.ConsumeTagSet("fargate", tag, []string{tag})

	c.addRuntimeTelemetryMetric("", nil)

	metricsByName := make(map[string][]string) // name → hosts
	for _, s := range c.series {
		metricsByName[s.Name] = append(metricsByName[s.Name], s.Host)
	}
	assert.ElementsMatch(t, metricsByName["otel.datadog_exporter.metrics.running"], []string{"my-hostname"})
	assert.ElementsMatch(t, metricsByName["otel.datadog_exporter.metrics.running.fargate"], []string{""})
	assert.Len(t, c.series, 2)
}

func TestAzureContainerAppsMetric(t *testing.T) {
	buildInfo := component.BuildInfo{}
	c := newTestCollectorConsumer(buildInfo)
	key := "sub-123/my-rg/my-app/replica-1"
	tags := []string{
		"replica_name:replica-1",
		"name:my-app",
		"subscription_id:sub-123",
		"resource_group:my-rg",
	}
	c.ConsumeTagSet("azurecontainerapps", key, tags)
	// Same key — should not duplicate
	c.ConsumeTagSet("azurecontainerapps", key, tags)
	c.addRuntimeTelemetryMetric("", nil)

	// Exactly one series total: the ACA metric only. The hostless fallback
	// emission of "otel.datadog_exporter.metrics.running" must be suppressed
	// here, the same way it is for Fargate-only sources, to avoid
	// double-counting a single ACA workload for billing.
	require.Len(t, c.series, 1, "expected exactly one series (ACA only, no stray hostless fallback metric)")
	found := c.series[0]
	assert.Equal(t, "otel.datadog_exporter.metrics.running.azurecontainerapps", found.Name)
	tagStrs := found.Tags.UnsafeToReadOnlySliceString()
	assert.Contains(t, tagStrs, "replica_name:replica-1")
	assert.Contains(t, tagStrs, "name:my-app")
	assert.Contains(t, tagStrs, "subscription_id:sub-123")
	assert.Contains(t, tagStrs, "resource_group:my-rg")
}
