// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// ContainersTelemetry represents the objects necessary to send metrics listing containers
type ContainersTelemetry struct {
	Sender        aggregator.Sender
	MetadataStore workloadmeta.Store
	IgnoreDDAgent bool
}

// NewContainersTelemetry returns a new ContainersTelemetry based on default/global objects
func NewContainersTelemetry() (*ContainersTelemetry, error) {
	sender, err := aggregator.GetDefaultSender()
	if err != nil {
		return nil, err
	}

	return &ContainersTelemetry{
		Sender:        sender,
		MetadataStore: workloadmeta.GetGlobalStore(),
	}, nil
}

func (c *ContainersTelemetry) ListRunningContainers() []*workloadmeta.Container {
	return c.MetadataStore.ListContainersWithFilter(workloadmeta.GetRunningContainers)
}

// ReportContainers sends the metrics about currently running containers
// This function is critical for CWS/CSPM metering. Please tread carefully.
func (c *ContainersTelemetry) ReportContainers(metricName string) {
	containers := c.ListRunningContainers()

	for _, container := range containers {
		if c.IgnoreDDAgent {
			value := container.EnvVars["DOCKER_DD_AGENT"]
			value = strings.ToLower(value)
			if value == "yes" || value == "true" {
				log.Debugf("ignoring container: name=%s id=%s image_id=%s", container.Name, container.ID, container.Image.ID)
				continue
			}
		}

		c.Sender.Gauge(metricName, 1.0, "", []string{"container_id:" + container.ID})
	}

	c.Sender.Commit()
}
