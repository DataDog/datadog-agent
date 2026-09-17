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
		seenTagSets:        make(map[tagSetKey][]string),
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

func TestExporterWorkloadMetrics(t *testing.T) {
	tags := []string{"version:1.0", "command:otelcontribcol", "instance:instance-1"}
	serie := exporterWorkloadMetrics("azureappservices", uint64(2e9), tags)

	assert.Equal(t, "otel.datadog_exporter.metrics.running.azureappservices", serie.Name)
	assert.Len(t, serie.Points, 1)
	assert.Equal(t, float64(2e9), serie.Points[0].Ts)
	assert.Equal(t, 1.0, serie.Points[0].Value)
	assert.Empty(t, serie.Host)
	assert.ElementsMatch(t, tags, serie.Tags.UnsafeToReadOnlySliceString())
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

func TestAddRuntimeTelemetryMetric_AzureAppServices(t *testing.T) {
	c := newTestCollectorConsumer(component.BuildInfo{})
	tags := []string{
		"instance:instance-1",
		"name:my-app",
		"subscription_id:sub-123",
		"resource_group:my-rg",
	}
	reordered := []string{
		"resource_group:my-rg",
		"subscription_id:sub-123",
		"name:my-app",
		"instance:instance-1",
	}
	c.ConsumeTagSet("azureappservices", tags)
	c.ConsumeTagSet("azureappservices", reordered)

	c.addRuntimeTelemetryMetric("", nil)

	assert.Len(t, c.series, 1)
	assert.Equal(t, "otel.datadog_exporter.metrics.running.azureappservices", c.series[0].Name)
	assert.ElementsMatch(t, tags, c.series[0].Tags.UnsafeToReadOnlySliceString())
}

func TestAddRuntimeTelemetryMetric_AzureAppServicesDedupKeyIsUnambiguous(t *testing.T) {
	c := newTestCollectorConsumer(component.BuildInfo{})
	first := []string{
		"instance:instance-1",
		"name:my-app",
		"resource_group:my-rg,resource_group:other-rg",
		"subscription_id:sub-123",
	}
	second := []string{
		"instance:instance-1",
		"name:my-app,resource_group:my-rg",
		"resource_group:other-rg",
		"subscription_id:sub-123",
	}

	// Joining either sorted tag set with commas produces the same string. They
	// must still identify two distinct App Service resources.
	c.ConsumeTagSet("azureappservices", first)
	c.ConsumeTagSet("azureappservices", second)
	c.addRuntimeTelemetryMetric("", nil)

	assert.Len(t, c.series, 2)
	var got [][]string
	for _, serie := range c.series {
		assert.Equal(t, "otel.datadog_exporter.metrics.running.azureappservices", serie.Name)
		got = append(got, serie.Tags.UnsafeToReadOnlySliceString())
	}
	assert.ElementsMatch(t, [][]string{first, second}, got)
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
