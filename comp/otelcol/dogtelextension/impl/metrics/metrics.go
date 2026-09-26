// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package metrics provides metric creation helpers for the dogtelextension.
package metrics

import (
	"time"

	"go.opentelemetry.io/collector/component"

	taggertags "github.com/DataDog/datadog-agent/comp/core/tagger/tags"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
)

// TagsFromBuildInfo returns a list of tags derived from buildInfo to be used when creating metrics.
func TagsFromBuildInfo(buildInfo component.BuildInfo) []string {
	var tags []string
	if buildInfo.Version != "" {
		tags = append(tags, "version:"+buildInfo.Version)
	}
	if buildInfo.Command != "" {
		tags = append(tags, "command:"+buildInfo.Command)
	}
	return tags
}

// CreateLivenessSerie creates a liveness metric serie to report that the dogtel extension is running.
func CreateLivenessSerie(hostname string, timestamp time.Time, tags []string) *metrics.Serie {
	timestampSeconds := time.Duration(timestamp.UnixNano()).Seconds()

	return &metrics.Serie{
		Name:           "otel.dogtel_extension.running",
		Points:         []metrics.Point{{Ts: timestampSeconds, Value: 1.0}},
		Tags:           tagset.NewCompositeTags(tags, nil),
		Host:           hostname,
		MType:          metrics.APIGaugeType,
		SourceTypeName: "otel.dogtel_extension",
		Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
}

// CreateFargateLivenessSerie creates a liveness metric serie to report that the dogtel extension
// is running in an ECS Fargate task. The task is identified by its ARN rather than a hostname,
// since Fargate tasks have no host identity.
func CreateFargateLivenessSerie(taskARN string, timestamp time.Time, tags []string) *metrics.Serie {
	timestampSeconds := time.Duration(timestamp.UnixNano()).Seconds()

	allTags := append([]string{taggertags.TaskARN + ":" + taskARN}, tags...)

	return &metrics.Serie{
		Name:           "otel.dogtel_extension.running.fargate",
		Points:         []metrics.Point{{Ts: timestampSeconds, Value: 1.0}},
		Tags:           tagset.NewCompositeTags(allTags, nil),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "otel.dogtel_extension",
		Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
}

// CreateAzureContainerAppsLivenessSerie creates a liveness metric serie tagged with the Azure
// Container Apps replica, app name, subscription ID and resource group instead of a hostname,
// since Azure Container Apps replicas have no host identity. Tag keys match the ones used by
// the community DD exporter's otel.datadog_exporter.metrics.running.azurecontainerapps metric.
// It returns nil unless name, subscriptionID and resourceGroup are all present, matching the DD
// exporter's behavior of never emitting this billing metric for a partially-identified resource.
func CreateAzureContainerAppsLivenessSerie(replica, name, subscriptionID, resourceGroup string, timestamp time.Time, tags []string) *metrics.Serie {
	if name == "" || subscriptionID == "" || resourceGroup == "" {
		return nil
	}

	timestampSeconds := time.Duration(timestamp.UnixNano()).Seconds()

	allTags := append([]string{
		"name:" + name,
		"subscription_id:" + subscriptionID,
		"resource_group:" + resourceGroup,
	}, tags...)
	if replica != "" {
		allTags = append(allTags, "replica:"+replica)
	}

	return &metrics.Serie{
		Name:           "otel.dogtel_extension.running.azurecontainerapps",
		Points:         []metrics.Point{{Ts: timestampSeconds, Value: 1.0}},
		Tags:           tagset.NewCompositeTags(allTags, nil),
		MType:          metrics.APIGaugeType,
		SourceTypeName: "otel.dogtel_extension",
		Source:         metrics.MetricSourceOpenTelemetryCollectorUnknown,
	}
}
