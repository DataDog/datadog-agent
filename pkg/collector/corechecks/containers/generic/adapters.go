// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package generic

import (
	"github.com/DataDog/datadog-agent/comp/workloadmeta"
)

// MetricsAdapter provides a way to change metrics and tags before sending them out
type MetricsAdapter interface {
	// AdaptTags can be used to change Tagger tags before submitting the metrics
	AdaptTags(tags []string, c *workloadmeta.Container) []string
	// AdaptMetrics can be used to change metrics (change name or value) before submitting the metric.
	AdaptMetrics(metricName string, value float64) (string, float64)
}

// ContainerAccessor abstracts away how to list all running containers
type ContainerAccessor interface {
	ListRunning() []*workloadmeta.Container
}

// MetadataContainerAccessor implements ContainerLister interface using Workload meta service
type MetadataContainerAccessor struct{}

// ListRunning returns all running containers
func (l MetadataContainerAccessor) ListRunning() []*workloadmeta.Container {
	// TODO(components): stop using globals, rely instead on injected component
	return workloadmeta.GetGlobalStore().ListContainersWithFilter(workloadmeta.GetRunningContainers)
}

// GenericMetricsAdapter implements MetricsAdapter API in a basic way.
// Adds `runtime` tag and do not change metrics.
type GenericMetricsAdapter struct{}

// AdaptTags adds a `runtime` tag for all containers
func (a GenericMetricsAdapter) AdaptTags(tags []string, c *workloadmeta.Container) []string {
	return append(tags, "runtime:"+string(c.Runtime))
}

// AdaptMetrics is a passthrough (does not change anything)
func (a GenericMetricsAdapter) AdaptMetrics(metricName string, value float64) (string, float64) {
	return metricName, value
}
