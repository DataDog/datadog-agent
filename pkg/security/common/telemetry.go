// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

type ContainersTelemetry struct {
	Sender        aggregator.Sender
	MetadataStore workloadmeta.Store
}

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

func (c *ContainersTelemetry) ReportContainers(metricName string) error {
	containers, err := c.MetadataStore.ListContainers()
	if err != nil {
		return err
	}

	for _, container := range containers {
		if container.State.Running {
			c.Sender.Gauge(metricName, 1.0, "", []string{"container_id:" + container.ID})
		}
	}

	c.Sender.Commit()

	return nil
}
