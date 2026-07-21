// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
)

func newTestCollectorConsumer(buildInfo component.BuildInfo) *collectorConsumer {
	s := &serializerConsumer{ipath: ossCollector}
	return &collectorConsumer{
		serializerConsumer: s,
		seenHosts:          make(map[string]struct{}),
		seenTags:           make(map[string]struct{}),
		buildInfo:          buildInfo,
		getPushTime:        func() uint64 { return uint64(2e9) },
	}
}

func TestExporterFargateMetrics(t *testing.T) {
	tags := []string{"version:1.0", "command:otelcontribcol"}
	serie := exporterFargateMetrics(uint64(2e9), tags)

	assert.Equal(t, "otel.datadog_exporter.metrics.running.fargate", serie.Name)
	assert.Equal(t, 1, len(serie.Points))
	assert.Equal(t, float64(2e9), serie.Points[0].Ts)
	assert.Equal(t, 1.0, serie.Points[0].Value)
	assert.Equal(t, "", serie.Host)
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
	c.ConsumeTag("task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc")

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
	c.ConsumeTag("task_arn:arn:aws:ecs:us-east-1:123:task/cluster/abc")

	c.addRuntimeTelemetryMetric("", nil)

	metricsByName := make(map[string][]string) // name → hosts
	for _, s := range c.series {
		metricsByName[s.Name] = append(metricsByName[s.Name], s.Host)
	}
	assert.ElementsMatch(t, metricsByName["otel.datadog_exporter.metrics.running"], []string{"my-hostname"})
	assert.ElementsMatch(t, metricsByName["otel.datadog_exporter.metrics.running.fargate"], []string{""})
	assert.Len(t, c.series, 2)
}
