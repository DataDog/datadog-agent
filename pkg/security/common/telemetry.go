// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// ContainersTelemetry represents the objects necessary to send metrics listing containers
type ContainersTelemetry struct {
	Sender        aggregator.Sender
	MetadataStore workloadmeta.Store
	Filter        *containers.Filter
}

// NewContainersTelemetry returns a new ContainersTelemetry based on default/global objects
func NewContainersTelemetry(respectFiltering bool) (*ContainersTelemetry, error) {
	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		return nil, err
	}

	ct := &ContainersTelemetry{
		Sender:        sender,
		MetadataStore: workloadmeta.GetGlobalStore(),
	}

	if respectFiltering {
		containerFilter, err := containers.GetSharedMetricFilter()
		if err != nil {
			log.Warnf("Can't get container include/exclude filter, no filtering will be applied: %v", err)
		} else {
			ct.Filter = containerFilter
		}
	}

	return ct, nil
}

// ReportContainers sends the metrics about currently running containers
// This function is critical for CWS/CSPM metering. Please tread carefully.
func (c *ContainersTelemetry) ReportContainers(metricName string) {
	containers := c.MetadataStore.ListContainersWithFilter(workloadmeta.GetRunningContainers)

	for _, container := range containers {
		if c.Filter != nil && c.Filter.IsExcluded(container.Name, container.Image.Name, container.Labels[kubernetes.CriContainerNamespaceLabel]) {
			continue
		}

		c.Sender.Gauge(metricName, 1.0, "", []string{"container_id:" + container.ID})
	}

	c.Sender.Commit()
}
