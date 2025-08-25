// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serializerexporter

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
)

// collectorConsumer is a consumer OSS collector uses to send metrics to the DataDog.
type collectorConsumer struct {
	*serializerConsumer
	seenHosts map[string]struct{}
	seenTags  map[string]struct{}
	buildInfo component.BuildInfo
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

	var tags []string
	tags = append(tags, buildTags...)
	for tag := range c.seenTags {
		tags = append(tags, tag)
	}
	runningMetrics := exporterDefaultMetrics("metrics", "", timestamp, tags)
	series = append(series, runningMetrics)

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
