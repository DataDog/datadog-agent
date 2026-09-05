// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"fmt"
	"slices"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/opentelemetry-mapping-go/otlp/attributes/source"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

type tagSetKey struct {
	metricSuffix string
	tags         string
}

// collectorConsumer is a consumer OSS collector uses to send metrics to the DataDog.
type collectorConsumer struct {
	*serializerConsumer
	seenHosts   map[string]struct{}
	seenTags    map[string]struct{}
	seenTagSets map[tagSetKey][]string
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

	var nonFargateTags []string
	for tag := range c.seenTags {
		if strings.HasPrefix(tag, string(source.AWSECSFargateKind)+":") {
			series = append(series, exporterFargateMetrics(timestamp, append(buildTags, tag)))
		} else {
			nonFargateTags = append(nonFargateTags, tag)
		}
	}
	if (len(c.seenHosts) > 0 && len(c.seenTags) == 0) || len(nonFargateTags) > 0 {
		tags := append(buildTags, nonFargateTags...)
		series = append(series, exporterDefaultMetrics("metrics", "", timestamp, tags))
	}

	for key, tags := range c.seenTagSets {
		allTags := append(slices.Clone(buildTags), tags...)
		series = append(series, exporterWorkloadMetrics(key.metricSuffix, timestamp, allTags))
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

// ConsumeTag implements the metrics.TagsConsumer interface.
func (c *collectorConsumer) ConsumeTag(tag string) {
	c.seenTags[tag] = struct{}{}
}

// ConsumeTagSet implements the metrics.TagSetConsumer interface.
func (c *collectorConsumer) ConsumeTagSet(metricSuffix string, tags []string) {
	sorted := slices.Clone(tags)
	slices.Sort(sorted)

	var dedupKey strings.Builder
	for _, tag := range sorted {
		fmt.Fprintf(&dedupKey, "%d:", len(tag))
		dedupKey.WriteString(tag)
	}
	key := tagSetKey{metricSuffix: metricSuffix, tags: dedupKey.String()}
	c.seenTagSets[key] = sorted
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

// exporterWorkloadMetrics creates a workload-specific exporter running metric.
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

// exporterFargateMetrics creates a built-in metric to report that a Fargate exporter is running.
func exporterFargateMetrics(timestamp uint64, tags []string) *metrics.Serie {
	return &metrics.Serie{
		Name: "otel.datadog_exporter.metrics.running.fargate",
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
