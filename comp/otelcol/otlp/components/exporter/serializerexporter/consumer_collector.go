// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"fmt"
	"slices"
	"strings"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
)

// tagSetKey namespaces a ConsumeTagSet dedup key by metricSuffix, so that two
// different workload types can never collide in seenTagSets even if their
// tag content happens to coincide. key is derived from tags themselves
// (sorted and joined) so that two calls with identical tags always dedup
type tagSetKey struct {
	metricSuffix string
	key          string
}

// collectorConsumer is a consumer OSS collector uses to send metrics to the DataDog.
type collectorConsumer struct {
	*serializerConsumer
	seenHosts   map[string]struct{}
	seenTagSets map[tagSetKey][]string // value: full "key:value" tag slice for the metric
	buildInfo   component.BuildInfo
	// getPushTime returns a Unix time in nanoseconds, representing the time pushing metrics.
	// It will be overwritten in tests.
	getPushTime func() uint64
}

var _ SerializerConsumer = (*collectorConsumer)(nil)

func (c *collectorConsumer) addRuntimeTelemetryMetric(_ string, languageTags []string) {
	timestamp := c.getPushTime()
	buildTags := tagsFromBuildInfo(c.buildInfo)
	series := c.series
	for host := range c.seenHosts {
		// Report the host as running
		runningMetric := exporterDefaultMetrics("metrics", host, timestamp, buildTags)
		series = append(series, runningMetric)
	}

	for k, tags := range c.seenTagSets {
		series = append(series, exporterWorkloadMetrics(k.metricSuffix, timestamp, append(buildTags, tags...)))
	}

	// Suppress the hostless fallback emission of "metrics.running" (no Host set)
	// when every signal seen was already attributed to a specific workload
	// type via seenTagSets, to avoid double-counting a single workload for
	// billing.
	if len(c.seenHosts) > 0 && len(c.seenTagSets) == 0 {
		series = append(series, exporterDefaultMetrics("metrics", "", timestamp, buildTags))
	}

	for _, lang := range languageTags {
		tags := append(buildTags, "language:"+lang) //nolint:gocritic
		runningMetric := exporterDefaultMetrics("runtime_metrics", "", timestamp, tags)
		series = append(series, runningMetric)
	}
	c.series = series
}

func (c *collectorConsumer) addTelemetryMetric(_ string, _ exporter.Settings, _ telemetry.Gauge) {
}

// ConsumeHost implements the metrics.HostConsumer interface.
func (c *collectorConsumer) ConsumeHost(host string) {
	c.seenHosts[host] = struct{}{}
}

// ConsumeTagSet implements the metrics.TagSetConsumer interface.
func (c *collectorConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)
	key := tagSetKey{metricSuffix: metricSuffix, key: strings.Join(sorted, ",")}
	c.seenTagSets[key] = tags
}

// exporterDefaultMetrics creates built-in metrics to report that an exporter is running
func exporterDefaultMetrics(exporterType string, hostname string, timestamp uint64, tags []string) *metrics.Serie {
	metrics := &metrics.Serie{
		Name: fmt.Sprintf("otel.datadog_exporter.%s.running", exporterType),
		Points: []metrics.Point{
			{
				Ts:    float64(timestamp),
				Value: 1.0,
			},
		},
		Host:   hostname,
		MType:  metrics.APIGaugeType,
		Tags:   tagset.CompositeTagsFromSlice(tags),
		Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
	return metrics
}

// exporterWorkloadMetrics creates a built-in metric to report that a
// workload-specific exporter (e.g. Fargate, Azure Container Apps) is
// running. The resulting metric name is
// "otel.datadog_exporter.metrics.running.<metricSuffix>".
func exporterWorkloadMetrics(metricSuffix string, timestamp uint64, tags []string) *metrics.Serie {
	return &metrics.Serie{
		Name: "otel.datadog_exporter.metrics.running." + metricSuffix,
		Points: []metrics.Point{
			{
				Ts:    float64(timestamp),
				Value: 1.0,
			},
		},
		Host:   "",
		MType:  metrics.APIGaugeType,
		Tags:   tagset.CompositeTagsFromSlice(tags),
		Source: metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
}

// tagsFromBuildInfo returns a list of tags derived from buildInfo to be used when creating metrics
func tagsFromBuildInfo(buildInfo component.BuildInfo) []string {
	var tags []string
	if buildInfo.Version != "" {
		tags = append(tags, "version:"+buildInfo.Version)
	}
	if buildInfo.Command != "" {
		tags = append(tags, "command:"+buildInfo.Command)
	}
	return tags
}
